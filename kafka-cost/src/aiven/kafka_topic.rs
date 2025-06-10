use std::{collections::HashMap, time::SystemTimeError};

use bigdecimal::ToPrimitive;
use chrono::{DateTime, TimeZone};
use color_eyre::eyre::{Result, bail};
use futures_util::future::try_join_all;
use serde::Deserialize;
use tracing::{info, trace};

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(rename_all = "UPPERCASE")]
pub enum AivenApiKafkaTopicState {
    #[default]
    Active,
    Configuring,
    Deleting,
}

#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafkaTopicPartition {
    /// In bytes according to the Aiven API
    pub remote_size: Option<u64>,
    /// In bytes according to the Aiven API
    pub size: u64,
}

// Need to subtract this:
//     https://api.aiven.io/doc/#tag/Service:_Kafka/operation/ServiceKafkaTieredStorageStorageUsageByTopic
// From this:
//     https://api.aiven.io/doc/#tag/Service:_Kafka/operation/ServiceKafkaTieredStorageStorageUsageTotal
// To find out how much storage a topic uses that isn't Tiered Storage
#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafkaTopic {
    #[serde(rename = "topic_name")]
    pub name: String,
    #[serde(deserialize_with = "tags_deserializer")]
    pub tags: HashMap<String, String>,
    #[serde(skip)]
    pub partitions: Vec<AivenApiKafkaTopicPartition>,
}
fn tags_deserializer<'de, D>(deserializer: D) -> Result<HashMap<String, String>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let input: Vec<HashMap<String, String>> = Deserialize::deserialize(deserializer)?;
    let result: HashMap<String, String> = input
        .into_iter()
        .map(|obj| (obj["key"].clone(), obj["value"].clone())) // Trust in Aiven never to refactor this structure...
        .collect();
    Ok(result)
    // todo!()
}

#[derive(Debug, Deserialize)]
struct AivenApiKafkaTopicSpecificData {
    #[serde(rename = "topic_name")]
    pub name: String,
    partitions: Vec<AivenApiKafkaTopicPartition>,
    #[serde(deserialize_with = "tags_deserializer")]
    pub tags: HashMap<String, String>,
}

impl AivenApiKafkaTopic {
    pub fn from_json_obj(input: &serde_json::Value) -> Result<Self> {
        let result = match serde_json::from_value(input.clone()) {
            Ok(line) => line,
            Err(e) => bail!(
                "Unable to parse json into expected data structure: {}: {:#?}",
                e,
                input
            ),
        };
        Ok(result)
    }

    pub async fn populate_with_partitions_belonging_to_topic(
        &mut self,
        reqwest_client: &reqwest::Client,
        cfg: &crate::Cfg,
        project_name: &str,
        kafka_name: &str,
    ) -> Result<Self> {
         let c =    reqwest::Client::builder()
        .https_only(true)
        .user_agent("very nais")
        .build()
        .map_err(color_eyre::eyre::Error::msg)?;

        use rand::prelude::*;
        use tokio::time::Duration;
        let mut rng = rand::rng();

        let some_millis: u16 = rng.random();
        let dur = Duration::from_millis(some_millis.to_u64().unwrap());
        let _ = tokio::time::sleep(dur);
        let t = chrono::offset::Local::now();
        let url = format!(
            "https://api.aiven.io/v1/project/{project_name}/service/{kafka_name}/topic/{}",
            self.name
        );
        let response = match c
            .get(&url)
            .bearer_auth(&cfg.aiven_api_token)
            .send()
            .await
        {
            Ok(r) => r,
            Err(e) => {
                let delta_t = chrono::offset::Local::now() - t;
                bail!("HTTP request failed with: {:?}, {}", e, delta_t)
            }
        };
        let response_status = &response.status();
        let Ok(mut response_body) =
            serde_json::from_str::<HashMap<String, serde_json::Value>>(&(response.text().await?))
        else {
            bail!("Unable to parse json returned from: GET {response_status} {url}")
        };
        let key = "topic";
        let Some(topic_json) = response_body.get_mut(key) else {
            bail!("missing field name:\n\t`{key}`\n\t\tGET {response_status} {url}")
        };

        let topic_data: AivenApiKafkaTopicSpecificData =
            match serde_json::from_value(topic_json.clone()) {
                Ok(t) => t,
                Err(e) => {
                    bail!(
                        "Unable to parse topic's partitions json from aiven: {}: {:?}",
                        e,
                        &topic_json
                    )
                }
            };
        self.partitions = topic_data.partitions;
        assert!(
            self.tags.iter().all(|(k, v)| &topic_data.tags[k] == v) && self.name == topic_data.name,
            "topic data doesn't match..."
        );
        let delta_t = chrono::offset::Local::now() - t;
        trace!("dt {}", delta_t);
        Ok(self.clone())
    }

    pub fn from_json_list(list: &[serde_json::Value]) -> Result<Vec<Self>> {
        list.iter().map(Self::from_json_obj).collect()
    }

    pub async fn get_all_from_aiven_api_by_kafka_instance(
        reqwest_client: &reqwest::Client,
        cfg: &crate::Cfg,
        project_name: &str,
        kafka_name: &str,
    ) -> Result<Vec<Self>> {
        let url =
            format!("https://api.aiven.io/v1/project/{project_name}/service/{kafka_name}/topic");
        let response = reqwest_client
            .get(&url)
            .bearer_auth(&cfg.aiven_api_token)
            .send()
            .await?;
        let response_status = &response.status();
        let Ok(mut response_body) =
            serde_json::from_str::<HashMap<String, serde_json::Value>>(&(response.text().await?))
        else {
            bail!("Unable to parse json returned from: GET {response_status} {url}")
        };
        let key = "topics";
        let Some(response_topics) = response_body
            .get_mut(key)
            .and_then(|topic| topic.as_array())
        else {
            bail!("missing field name:\n\t`{key}`\n\t\tGET {response_status} {url}")
        };

        info!(
            "Populating {} topics with partitions for {project_name} - {kafka_name}",
            response_topics.len()
        );

        use futures::stream::{self, StreamExt, TryStreamExt};

        // 1274 threads
        try_join_all(
            Self::from_json_list(response_topics)?
                .iter_mut()
                .map(|topic| {
                    topic.populate_with_partitions_belonging_to_topic(
                        reqwest_client,
                        cfg,
                        project_name,
                        kafka_name,
                    )
                }),
        )
        .await
    }
}
