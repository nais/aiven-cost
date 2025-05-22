use anyhow::{Result, bail};
use gcp_bigquery_client::Client;
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
    let Ok(response_body) =
        serde_json::from_str::<HashMap<String, serde_json::Value>>(&response.text().await?)
    else {
        bail!("Unable to parse json returned from: GET {url}")
    };
    let Some(response_invoices) = response_body
        .get("invoices")
        .and_then(|invoices| invoices.as_array())
    else {
        bail!("Aiven's BillingGroup invoices API return doesn't follow expected structure")
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
    dbg!(&response);
    let Ok(response_body) =
        serde_json::from_str::<HashMap<String, serde_json::Value>>(&response.text().await?)
    else {
        bail!("Unable to parse json returned from: GET {url}")
    };
    dbg!(&response_body);
    let Some(response_invoice_lines) = response_body
        .get("lines")
        .and_then(|invoices| invoices.as_array())
    else {
        bail!("Aiven's BillingGroup invoice's lines API return doesn't follow expected structure")
    };
    Ok(response_invoice_lines.to_vec())
}

mod aiven;
use aiven::{AivenInvoice, AivenInvoiceState};

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new()?;
    let bigquery_client = Client::from_application_default_credentials().await?;
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

    for invoice in unpaid_invoices {
        let invoice_lines_response =
            get_aiven_billing_group_invoice_list(&aiven_client, &cfg, &invoice.id).await?;
        dbg!(&invoice_lines_response);

        // TODO:
        // 	invoiceLines, err := aivenClient.GetInvoiceLines(ctx, invoice)
        // 	if err != nil {
        // 		return fmt.Errorf("failed to get invoice details for invoice %s: %w", invoice.InvoiceId, err)
        // 	}

        // 	err = bqClient.InsertCostItems(ctx, invoiceLines)
        // 	if err != nil {
        // 		return fmt.Errorf("failed to insert cost item into bigquery: %w", err)
        // 	}
    }

    Ok(())
}
