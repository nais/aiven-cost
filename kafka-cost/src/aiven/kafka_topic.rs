use std::collections::HashMap;

use color_eyre::eyre::{Result, bail};
use futures::stream::{self, StreamExt};
use serde::Deserialize;
use tracing::info;

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
    pub remote_size: Option<f64>,
    /// In bytes according to the Aiven API
    pub size: f64,
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
pub fn tags_deserializer<'de, D>(deserializer: D) -> Result<HashMap<String, String>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let input: Vec<HashMap<String, String>> = Deserialize::deserialize(deserializer)?;
    let result: HashMap<String, String> = input
        .into_iter()
        .map(|obj| (obj["key"].clone(), obj["value"].clone())) // Trust in Aiven never to refactor this structure...
        .collect();
    Ok(result)
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
        let url = format!(
            "https://api.aiven.io/v1/project/{project_name}/service/{kafka_name}/topic/{}",
            self.name
        );

        let response = match reqwest_client
            .get(&url)
            .bearer_auth(&cfg.aiven_api_token)
            .send()
            .await
        {
            Ok(r) => r,
            Err(e) => {
                bail!("HTTP request failed with: {:?}", e)
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
        let Some(response_topics) = response_body
            .get_mut("topics")
            .and_then(|topic| topic.as_array())
        else {
            bail!("missing field name:\n\t`topics` \n\t\tGET {response_status} {url}")
        };

        info!(
            "Populating '{project_name}'s kafka({kafka_name}) {} topics' partitions",
            response_topics.len()
        );

        let mut topics_with_no_partitions = Self::from_json_list(response_topics)?;
        let topics_with_partitions = stream::iter(topics_with_no_partitions.iter_mut())
            .map(|topic| {
                topic.populate_with_partitions_belonging_to_topic(
                    reqwest_client,
                    cfg,
                    project_name,
                    kafka_name,
                )
            })
            .buffer_unordered(5) // ten is too many against aiven. Five works. idk about the other numbers in [5,10]
            .collect::<Vec<_>>()
            .await;

        Ok(topics_with_partitions
            .into_iter()
            .filter_map(Result::ok)
            .collect())
    }
}
