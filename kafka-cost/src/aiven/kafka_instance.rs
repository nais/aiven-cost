use color_eyre::eyre::Result;
use serde::Deserialize;

use crate::Cfg;
use crate::aiven::kafka_topic::AivenApiKafkaTopic;

#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafka {
    #[serde(skip)]
    pub topics: Vec<AivenApiKafkaTopic>,
}

impl AivenApiKafka {
    pub async fn populate_from_aiven_api(
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
        project_name: &str,
        service_name: &str,
    ) -> Result<Self> {
        Ok(Self {
            topics: AivenApiKafkaTopic::get_all_from_aiven_api_by_kafka_instance(
                reqwest_client,
                cfg,
                project_name,
                service_name,
            )
            .await?,
        })
    }
}
