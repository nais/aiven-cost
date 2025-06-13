use std::{collections::HashMap, fmt::Display};

use chrono::{DateTime, Utc};
use color_eyre::eyre::{Result, bail};
use serde::Deserialize;
use serde::de::{self, Deserializer};
use tracing::info;

mod kafka_instance;
mod kafka_topic;
pub use self::kafka_instance::AivenApiKafka;
pub use self::kafka_topic::AivenApiKafkaTopic;
use crate::Cfg;

#[derive(Debug, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum AivenInvoiceState {
    Paid,
    Mailed,
    Estimate,
}

impl Display for AivenInvoiceState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Paid => write!(f, "paid"),
            Self::Mailed => write!(f, "mailed"),
            Self::Estimate => write!(f, "estimate"),
        }
    }
}

#[derive(Debug, Deserialize)]
pub struct AivenInvoice {
    #[serde(rename(deserialize = "invoice_number"))]
    pub id: String,
    pub state: AivenInvoiceState,
    pub period_begin: DateTime<Utc>,
}

impl AivenInvoice {
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

    pub fn from_json_list(list: &[serde_json::Value]) -> Result<Vec<Self>> {
        list.iter().map(Self::from_json_obj).collect()
    }

    pub async fn from_aiven_api(aiven_client: &reqwest::Client, cfg: &Cfg) -> Result<Vec<Self>> {
        let url = format!(
            "https://api.aiven.io/v1/billing-group/{}/invoice",
            cfg.billing_group_id
        );
        let response = aiven_client
            .get(&url)
            .bearer_auth(&cfg.aiven_api_token)
            .send()
            .await?;
        let response_status = &response.status();
        let Ok(response_body) =
            serde_json::from_str::<HashMap<String, serde_json::Value>>(&response.text().await?)
        else {
            bail!("Unable to parse json returned from: GET {response_status} {url}")
        };
        let Some(response_invoices) = response_body
            .get("invoices")
            .and_then(|invoices| invoices.as_array())
        else {
            bail!("missing field name:\n\t`invoices`\n\t\tGET {response_status} {url}")
        };
        info!(
            "Collected {} invoice(s) from Aiven's API",
            &response_invoices.len()
        );
        Self::from_json_list(response_invoices)
    }
}
#[derive(Clone, Debug, Default, Deserialize, PartialEq, Eq)]
pub enum KafkaInvoiceLineCostType {
    #[default]
    Base,
    TieredStorage,
}

fn string_to_f64<'de, D>(deserializer: D) -> Result<f64, D::Error>
where
    D: Deserializer<'de>,
{
    let s = String::deserialize(deserializer)?;
    s.parse::<f64>().map_err(de::Error::custom)
}

#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafkaInvoiceLine {
    #[serde(skip)]
    pub cost_type: KafkaInvoiceLineCostType,
    pub service_name: String,
    pub project_name: String,
    pub invoice_id: String,
    pub invoice_state: String,
    #[serde(skip)]
    pub kafka_instance: AivenApiKafka,
    #[serde(deserialize_with = "string_to_f64")]
    pub line_total_local: f64,
    pub timestamp_begin: DateTime<Utc>,
}

impl AivenApiKafkaInvoiceLine {
    pub fn from_json_obj(obj: &serde_json::Value) -> Result<Self> {
        let mut result: Self = match serde_json::from_value(obj.clone()) {
            Ok(line) => line,
            Err(e) => bail!(
                "Unable to parse json into expected data structure: {}: {:#?}",
                e,
                obj
            ),
        };
        let key = "description";
        let Some(description) = obj.get(key).and_then(serde_json::Value::as_str) else {
            bail!("missing field name:\n\t`{key}`\n\t\t{obj:#?}")
        };
        result.cost_type = KafkaInvoiceLineCostType::Base;
        if description
            .to_lowercase()
            .contains(": kafka tiered storage")
        {
            result.cost_type = KafkaInvoiceLineCostType::TieredStorage;
        }
        Ok(result)
    }

    pub fn from_json_list(list: &[&mut serde_json::Value]) -> Result<Vec<Self>> {
        list.iter().map(|obj| Self::from_json_obj(obj)).collect()
    }

    pub async fn populate_with_topics_from_aiven_api(
        mut self,
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
    ) -> Result<Self> {
        self.kafka_instance
            .populate_with_topics_from_aiven_api(reqwest_client, cfg)
            .await?;
        Ok(self)
    }

    pub async fn populate_with_tags_from_aiven_api(
        &mut self,
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
    ) -> Result<Self> {
        info!(
            "{}: getting tags for {} - {}",
            self.invoice_id, self.project_name, self.service_name
        );
        self.kafka_instance = AivenApiKafka::from_aiven_api(
            reqwest_client,
            cfg,
            &self.project_name,
            &self.invoice_state,
            &self.service_name,
        )
        .await?;
        Ok(self.to_owned())
    }

    pub async fn from_aiven_api(
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,

        invoice_id: &str,
        invoice_state: &AivenInvoiceState,
    ) -> Result<Vec<Self>> {
        let url = format!(
            "https://api.aiven.io/v1/billing-group/{}/invoice/{}/lines",
            cfg.billing_group_id, invoice_id,
        );
        let response = reqwest_client
            .get(&url)
            .bearer_auth(&cfg.aiven_api_token)
            .send()
            .await?;
        let response_status = &response.status();
        let Ok(response_body) =
            serde_json::from_str::<HashMap<String, serde_json::Value>>(&(response.text().await?))
        else {
            bail!("Unable to parse json returned from: GET {response_status} {url}")
        };
        let Some(response_invoice_lines) = response_body
            .get("lines")
            .and_then(|invoices| invoices.as_array())
        else {
            bail!("missing field name:\n\t`lines`\n\t\tGET {response_status} {url}")
        };

        // Keep only Kafka related invoices
        let mut copy = response_invoice_lines.clone();
        let kafka_invoice_lines: Vec<_> = copy
            .iter_mut()
            .filter(|line| {
                line.get("service_type").and_then(serde_json::Value::as_str) == Some("kafka")
            })
            .map(|json| {
                if let Some(map) = json.as_object_mut() {
                    map.insert(
                        "invoice_id".to_string(),
                        serde_json::to_value(invoice_id.to_string())?,
                    );
                    map.insert(
                        "invoice_state".to_string(),
                        serde_json::to_value(invoice_state.to_string())?,
                    );
                }

                Ok(json)
            })
            // .cloned()
            .collect::<Result<Vec<_>>>()?;

        info!(
            "invoice id {invoice_id} had {} line(s), of which {:?} were Kafka related",
            &response_invoice_lines.len(),
            &kafka_invoice_lines.len()
        );

        Self::from_json_list(&kafka_invoice_lines)
    }
}

pub async fn get_tags_of_aiven_service(
    reqwest_client: &reqwest::Client,
    cfg: &Cfg,
    project_name: &str,
    service_name: &str,
) -> Result<Option<serde_json::Value>> {
    let url =
        format!("https://api.aiven.io/v1/project/{project_name}/service/{service_name}/tags",);
    let response = reqwest_client
        .get(&url)
        .bearer_auth(&cfg.aiven_api_token)
        .send()
        .await?;
    let response_status = &response.status();
    let Ok(response_body) =
        serde_json::from_str::<HashMap<String, serde_json::Value>>(&(response.text().await?))
    else {
        bail!("unable to parse json returned from: GET {response_status} {url}")
    };

    if response_status == &reqwest::StatusCode::NOT_FOUND {
        info!("The Kafka instance {}, - does not exist", service_name);
        // a 404 is not a failure here, it just means it didn't exist when we asked about it
        // it could have been deleted previously but still exist on the invoice
        return Ok(None);
    }

    let Some(response_tags) = response_body.get("tags") else {
        bail!("missing field name:\n\t`tags`\n\t\tGET {response_status} {url}")
    };
    Ok(Some(response_tags.clone()))
}
