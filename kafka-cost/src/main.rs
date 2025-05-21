use anyhow::{Result, bail};
use bigdecimal::{BigDecimal, Num};
use serde_json::Value;
use std::collections::HashMap;
use tracing::info;

pub fn init_tracing_subscriber() -> Result<()> {
    tracing_subscriber::fmt().init();
    Ok(())
}

fn client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .https_only(true)
        .user_agent("nais-kafka-cost")
        .build()
        .map_err(anyhow::Error::msg)
}

#[derive(Debug)]
pub struct Cfg {
    pub aiven_api_token: String,
    pub billing_group_id: String,
    pub bigquery_project_id: String,
    pub bigquery_dataset: String,
    pub bigquery_table: String,
}

impl Cfg {
    fn new() -> Result<Self> {
        Ok(Self {
            // Aiven stuff
            aiven_api_token: std::env::var("AIVEN_API_TOKEN").expect("api token"),
            billing_group_id: "7d14362d-1e2a-4864-b408-1cc631bc4fab".to_owned(),
            // BQ stuff
            bigquery_project_id: "nais-io".to_owned(),
            bigquery_dataset: "kafka_team_cost".to_owned(),
            bigquery_table: "kafka_team_cost".to_owned(),
        })
    }
}

#[derive(Debug, PartialEq, Eq)]
enum AivenInvoiceState {
    Paid,
    Mailed,
    Estimate,
}

impl AivenInvoiceState {
    fn from_json_obj(obj: &serde_json::Value) -> Result<Self> {
        let Some(json_string) = obj.as_str() else {
            bail!("Unable to parse as str: {obj:?}")
        };
        let result = match json_string {
            "paid" => Self::Paid,
            "mailed" => Self::Mailed,
            "estimate" => Self::Estimate,
            _ => bail!("Unknown state: '{json_string}'"),
        };
        Ok(result)
    }
}

#[derive(Debug)]
struct AivenInvoice {
    pub(crate) id: String,
    pub(crate) total_inc_vat: BigDecimal,
    pub(crate) state: AivenInvoiceState,
}

impl AivenInvoice {
    fn from_json_obj(obj: &serde_json::Value) -> Result<Self> {
        let total_inc_vat_key = "total_inc_vat";
        let Some(Some(total_inc_vat_json)) = obj.get(total_inc_vat_key).map(|s| s.as_str()) else {
            bail!("Unable to find `{total_inc_vat_key}` in: {obj:?}")
        };
        // TODO: Verify this is correct, and we're not supposed to turn it into float first or something
        let Ok(total_inc_vat) = BigDecimal::from_str_radix(total_inc_vat_json, 10) else {
            bail!("Unable to parse as bigdecimal: '{total_inc_vat_json}'")
        };

        let invoice_number_key = "invoice_number";
        let Some(Some(id)) = obj.get(invoice_number_key).map(|s| s.as_str()) else {
            bail!("Unable to parse/find invoice's '{invoice_number_key}': {obj:?}")
        };

        let state_key = "state";
        let Some(state_json) = obj.get(state_key) else {
            bail!("Unable to find invoice's '{state_key}': {obj:?}")
        };
        let state = AivenInvoiceState::from_json_obj(state_json)?;

        Ok(Self {
            id: id.to_string(),
            total_inc_vat,
            state,
        })
    }

    fn from_json_list(list: &[serde_json::Value]) -> Result<Vec<Self>> {
        Ok(list.iter().flat_map(Self::from_json_obj).collect())
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new()?;

    let aiven_client = client()?;
    let response = aiven_client
        .get(&format!(
            "https://api.aiven.io/v1/billing-group/{}/invoice",
            cfg.billing_group_id
        ))
        .bearer_auth(cfg.aiven_api_token)
        .send()
        .await?;
    let response_body: HashMap<String, Value> =
        serde_json::from_str(&response.text().await?).unwrap();
    let Some(response_invoices) = response_body
        .get("invoices")
        .and_then(|invoices| invoices.as_array())
    else {
        bail!("Aiven's BillingGroup invoices API return doesn't follow expected structure")
    };

    let invoices: Vec<_> = AivenInvoice::from_json_list(response_invoices)?;
    let unpaid_invoices: Vec<_> = invoices
        .iter()
        .filter(|i| i.state != AivenInvoiceState::Paid)
        .collect();
    dbg!(unpaid_invoices);

    Ok(())
}
