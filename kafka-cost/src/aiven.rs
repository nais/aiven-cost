use std::collections::HashMap;

use anyhow::{Result, bail};
use bigdecimal::BigDecimal;
use serde::Deserialize;
use tracing::info;

mod kafka_instance;
mod kafka_topic;
pub use self::kafka_instance::AivenApiKafka;
use crate::Cfg;

#[derive(Debug, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum AivenInvoiceState {
    Paid,
    Mailed,
    Estimate,
}

#[derive(Debug, Deserialize)]
pub struct AivenInvoice {
    #[serde(rename(deserialize = "invoice_number"))]
    pub id: String,
    // pub total_inc_vat: String, // TODO: Comment back in only if we want to double-check that all the lines add up correctly
    pub state: AivenInvoiceState,
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
            bail!(
                "Aiven's API returns json with missing field name:\n\t`invoices`\n\t\tGET {response_status} {url}"
            )
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

#[derive(Clone, Debug, Default, Deserialize)]
pub struct AivenApiKafkaInvoiceLine {
    #[serde(skip)]
    pub cost_type: KafkaInvoiceLineCostType,
    pub service_name: String,
    pub project_name: String,
    #[serde(skip)]
    pub kafka_instance: AivenApiKafka,
    pub line_total_local: BigDecimal,
    pub local_currency: String,
    pub invoice_id: String,
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
            bail!("Aiven's API returns json with missing field name:\n\t`{key}`\n\t\t{obj:#?}")
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
        self.kafka_instance = AivenApiKafka::from_aiven_api(
            reqwest_client,
            cfg,
            &self.project_name,
            &self.service_name,
        )
        .await?;
        Ok(self.to_owned())
    }

    pub async fn from_aiven_api(
        reqwest_client: &reqwest::Client,
        cfg: &Cfg,
        invoice_id: &str,
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
            bail!(
                "Aiven's API returns json with missing field name:\n\t`lines`\n\t\tGET {response_status} {url}"
            )
        };
        // dbg!(&response_invoice_lines.len());

        // Keep only Kafka related invoices
        let mut copy = response_invoice_lines.clone();
        let kafka_invoice_lines: Vec<_> = copy
            .iter_mut()
            .filter(|line| {
                line.get("service_type").and_then(serde_json::Value::as_str) == Some("kafka")
            })
            .map(|json| {
                if let Some(map) = json.as_object_mut() {
                    map.insert("invoice_id".to_string(), serde_json::to_value(invoice_id)?);
                }

                Ok(json)
            })
            // .cloned()
            .collect::<Result<Vec<_>>>()?;

        info!(
            "Invoice ID {invoice_id} had {} line(s), of which {:?} were kafka related",
            &response_invoice_lines.len(),
            &kafka_invoice_lines
        );

        Self::from_json_list(&kafka_invoice_lines)
    }
}

pub async fn get_tags_of_aiven_service(
    reqwest_client: &reqwest::Client,
    cfg: &Cfg,
    project_name: &str,
    service_name: &str,
) -> Result<serde_json::Value> {
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
        bail!("Unable to parse json returned from: GET {response_status} {url}")
    };
    let Some(response_tags) = response_body.get("tags") else {
        bail!(
            "Aiven's API returns json with missing field name:\n\t`tags`\n\t\tGET {response_status} {url}"
        )
    };
    Ok(response_tags.clone())
}
