use anyhow::{Result, bail};
use chrono::DateTime;
use gcp_bigquery_client::{
    Client, model::table_data_insert_all_request::TableDataInsertAllRequest,
};
use std::collections::HashMap;
use tracing::info;

pub fn init_tracing_subscriber() -> Result<()> {
    tracing_subscriber::fmt().init();
    Ok(())
}

const USER_AGENT: &str = "nais.io-kafka-cost";

fn client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .https_only(true)
        .user_agent(USER_AGENT)
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
            billing_group_id: "7d14362d-1e2a-4864-b408-1cc631bc4fab".into(),
            // BQ stuff
            bigquery_project_id: "nais-io".into(),
            bigquery_dataset: "kafka_team_cost".into(),
            bigquery_table: "kafka_team_cost".into(),
        })
    }
}

async fn get_aiven_billing_group(
    reqwest_client: &reqwest::Client,
    cfg: &Cfg,
) -> Result<Vec<serde_json::Value>> {
    let url = format!(
        "https://api.aiven.io/v1/billing-group/{}/invoice",
        cfg.billing_group_id
    );
    let response = reqwest_client
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
    Ok(response_invoices.to_vec())
}

async fn get_aiven_billing_group_invoice_list(
    reqwest_client: &reqwest::Client,
    cfg: &Cfg,
    invoice_id: &str,
) -> Result<Vec<serde_json::Value>> {
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
    let Ok(response_body) = serde_json::from_str::<HashMap<String, serde_json::Value>>(
        &(&response.text().await?).clone(),
    ) else {
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
    Ok(response_invoice_lines.to_vec())
}

async fn get_aiven_service_tags(
    reqwest_client: &reqwest::Client,
    cfg: &Cfg,
    project_id: &str,
    service_name: &str,
) -> Result<serde_json::Map<String, serde_json::Value>> {
    let url = format!("https://api.aiven.io/v1/project/{project_id}/service/{service_name}/tags");
    let response = reqwest_client
        .get(&url)
        .bearer_auth(&cfg.aiven_api_token)
        .send()
        .await?;
    let response_status = &response.status();
    let Ok(response_body) = serde_json::from_str::<HashMap<String, serde_json::Value>>(
        &(&response.text().await?).clone(),
    ) else {
        bail!("Unable to parse json returned from: GET {response_status} {url}")
    };
    dbg!(&response_body);
    let Some(response_tags) = response_body.get("tags").and_then(|tags| tags.as_object()) else {
        bail!(
            "Aiven's API returns json with missing field name:\n\t`tags`\n\t\tGET {response_status} {url}"
        )
    };

    Ok(response_tags.to_owned())
}

mod aiven;
use aiven::{AivenInvoice, AivenInvoiceState};

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new()?;
    let aiven_client = client()?;

    let invoices: Vec<_> =
        AivenInvoice::from_json_list(&get_aiven_billing_group(&aiven_client, &cfg).await?)?;
    let unpaid_invoices: Vec<_> = invoices
        .iter()
        .filter(|i| i.state != AivenInvoiceState::Paid)
        .collect();
    info!(
        "fetch invoice details from aiven and insert into bigquery for {} invoices of a total of {}",
        unpaid_invoices.len(),
        invoices.len()
    );
    dbg!(&unpaid_invoices);

    let bigquery_client = Client::from_application_default_credentials().await?;
    let mut insert_request = TableDataInsertAllRequest::new();
    for invoice in unpaid_invoices {
        let invoice_lines_response =
            get_aiven_billing_group_invoice_list(&aiven_client, &cfg, &invoice.id).await?;
        // dbg!(&invoice_lines_response);

        let mut bq_rows = Vec::new();
        for line in invoice_lines_response {
            let project_name_key = "project_name";
            let Some(Some(project_id)) = line.get(project_name_key).map(|s| s.as_str()) else {
                bail!("Unable to find `{project_name_key}` in invoice_line: {line:?}")
            };
            let service_name_key = "service_name";
            let Some(Some(service_name)) = line.get(service_name_key).map(|s| s.as_str()) else {
                bail!("Unable to find `{service_name_key}` in invoice_line: {line:?}")
            };
            let service_tags =
                get_aiven_service_tags(&aiven_client, &cfg, project_id, service_name).await?;

            let timestamp_begin_key = "timestamp_begin";
            let Some(Some(timestamp_begin)) = line.get(timestamp_begin_key).map(|s| s.as_str())
            else {
                bail!("Unable to find `{timestamp_begin_key}` in invoice_line: {line:?}")
            };
            let time_invoice_line_begins = DateTime::parse_from_rfc3339(timestamp_begin)?;
            // TODO: Continue w/extracting data from the invoice line, i.e. build up data source for what's to be sent to BQ
            todo!()

            // TODO: Map from whatever AivenInvoiceLine struct we end up with to a BQ struct we add to the `bq_rows` vec
        }

        insert_request.add_rows(bq_rows)?;
    }

    // TODO: Create table if it does not exist
    bigquery_client
        .tabledata()
        .insert_all(
            &cfg.bigquery_project_id,
            &cfg.bigquery_dataset,
            &cfg.bigquery_table,
            insert_request,
        )
        .await?;

    Ok(())
}
