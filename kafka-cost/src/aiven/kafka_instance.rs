use anyhow::{Result, bail};
use serde::Deserialize;
use tracing::info;

use crate::Cfg;
use crate::aiven::get_tags_of_aiven_service;
use crate::aiven::kafka_topic::AivenApiKafkaTopic;

#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafka {
    #[serde(skip)]
    pub topics: Vec<AivenApiKafkaTopic>,
    /// Fetched from tags on the Kafka service's Aiven API
    pub tenant: String,
    #[serde(skip)]
    pub invoice_type: String,
    /// Fetched from tags on the Kafka service's Aiven API
    pub environment: String,
    #[serde(skip)]
    pub project_name: String,
    #[serde(skip)]
    pub service_name: String,
}
impl AivenApiKafka {
    pub fn from_json_obj(
        input: &serde_json::Value,
        project_name: &str,
        invoice_type: &str,
        service_name: &str,
    ) -> Result<Self> {
        let mut result: Self = match serde_json::from_value(input.clone()) {
            Ok(line) => line,
            Err(e) => bail!(
                "Unable to parse json into expected data structure: {}: {:#?}",
                e,
                input
            ),
        };
        result.project_name = project_name.to_string();
        result.invoice_type = invoice_type.to_string();
        result.service_name = service_name.to_string();
        Ok(result)
    }

    pub async fn from_aiven_api(
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
        project_name: &str,
        invoice_type: &str,
        service_name: &str,
    ) -> Result<Self> {

        let result = Self::from_json_obj(
            &get_tags_of_aiven_service(reqwest_client, cfg, project_name, service_name).await?.unwrap_or_else(|| serde_json::json!({"tenant": "", "environment": ""})),
            project_name,
            invoice_type,
            service_name,
        )?;

        assert!(
            result.project_name == project_name && service_name == result.service_name,
            "Project and Service names don't match expected"
        );
        Ok(result)
    }

    pub async fn populate_with_topics_from_aiven_api(
        &mut self,
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
    ) -> Result<&Self> {
        self.topics = AivenApiKafkaTopic::get_all_from_aiven_api_by_kafka_instance(
            reqwest_client,
            cfg,
            &self.project_name,
            &self.service_name,
        )
        .await?;
        info!(
            "Found {} topics for {}'s '{}' environment kafka instance '{}'",
            self.topics.len(),
            self.tenant,
            self.environment,
            self.service_name
        );
        Ok(self)
    }
}
