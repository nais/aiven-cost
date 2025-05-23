use anyhow::{Result, bail};
use bigdecimal::{BigDecimal, FromPrimitive, Num, ParseBigDecimalError, Zero};
use chrono::DateTime;
use gcp_bigquery_client::{
    Client, model::table_data_insert_all_request::TableDataInsertAllRequest,
};
use std::collections::{HashMap, HashSet};
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

#[derive(Debug, Clone)]
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


async fn get_aiven_topic_details(aiven_client: &reqwest::Client, cfg: &Cfg, project_name: &str, service_name: &str, topic_name: &str) -> Result<serde_json::Map<String, serde_json::Value>> {
        let url = format!(
            "https://api.aiven.io/v1/project/{project_name}/service/{service_name}/topic/{topic_name}"
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
    let Some(response_topic) = response_body
        .get("topic")
        .and_then(|topic| topic.as_object())
    else {
        bail!(
            "Aiven's API returns json with missing field name:\n\t`invoices`\n\t\tGET {response_status} {url}"
        )
    };
    Ok(response_topic.to_owned())
}

async fn get_aiven_billing_group(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
) -> Result<Vec<serde_json::Value>> {
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

async fn get_aiven_kafka_topics(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
    project_name: &str,
    service_name: &str,
) -> Result<Vec<serde_json::Value>> {
    let url =
        format!("https://api.aiven.io/v1/project/{project_name}/service/{service_name}/topic");
    let response = aiven_client
        .get(&url)
        .bearer_auth(&cfg.aiven_api_token)
        .send()
        .await?;
    let response_status = &response.status();
    let Ok(mut response_body) = serde_json::from_str::<HashMap<String, serde_json::Value>>(
        &(&response.text().await?).clone(),
    ) else {
        bail!("Unable to parse json returned from: GET {response_status} {url}")
    };
    let Some(response_topics) = response_body
        .get_mut("topics")
        .and_then(|topic| topic.as_array())
    else {
        bail!(
            "Aiven's API returns json with missing field name:\n\t`tags`\n\t\tGET {response_status} {url}"
        )
    };

    let mut rts = response_topics.to_owned();

    rts.iter_mut().for_each(|t| {
        let mut tt = t.as_object_mut();
        tt.as_mut().map(|ttt| {
            ttt.insert(
                "service_name".to_string(),
                serde_json::Value::String(service_name.to_string()),
            );
            ttt.insert(
                "project_name".to_string(),
                serde_json::Value::String(project_name.to_string()),
            );
        });
    });
    Ok(rts)
}

async fn get_aiven_service_tags(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
    project_name: &str,
    service_name: &str,
) -> Result<serde_json::Map<String, serde_json::Value>> {
    let url = format!("https://api.aiven.io/v1/project/{project_name}/service/{service_name}/tags");
    let response = aiven_client
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
    let Some(response_tags) = response_body.get("tags").and_then(|tags| tags.as_object()) else {
        bail!(
            "Aiven's API returns json with missing field name:\n\t`tags`\n\t\tGET {response_status} {url}"
        )
    };

    let mut tags = response_tags.to_owned();
    tags.insert(
        "service_name".to_string(),
        serde_json::Value::String(service_name.to_string()),
    );
    tags.insert(
        "project_name".to_string(),
        serde_json::Value::String(project_name.to_string()),
    );
    Ok(tags)
}

mod aiven;
use aiven::{AivenInvoice, AivenInvoiceState, KafkaLine};

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new()?;
    let aiven_client = client()?;
    let bigquery_client = Client::from_application_default_credentials().await?;

    let (topics, tags, kafka_lines) = extract(&aiven_client, &cfg).await?;
    let data = transform(topics, tags, kafka_lines);

    Ok(())
}

async fn extract(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
) -> Result<(
    Vec<serde_json::Value>,
    Vec<serde_json::Map<String, serde_json::Value>>,
    Vec<KafkaLine>,
)> {
    let invoices: Vec<_> =
        AivenInvoice::from_json_list(&get_aiven_billing_group(&aiven_client, &cfg).await?)?;
    let unpaid_invoices: Vec<_> = invoices
        .iter()
        .filter(|i| i.state != AivenInvoiceState::Paid) // sus
        .collect();
    info!(
        "fetch invoice details from aiven and insert into bigquery for {} invoices of a total of {}",
        unpaid_invoices.len(),
        invoices.len()
    );

    dbg!(&unpaid_invoices);

    let mut kafka_cost = Vec::new();
    let mut tags = Vec::new();
    let mut topics = Vec::new();
    for invoice in unpaid_invoices {
        let invoice_lines_response =
            get_aiven_billing_group_invoice_list(&aiven_client, &cfg, &invoice.id).await?;
        dbg!(&invoice_lines_response);
        for line in invoice_lines_response {
            if let Some("kafka") = line.get("service_type").and_then(|s| s.as_str()) {
                dbg!(&line);
                let project_name_key = "project_name";
                let Some(project_name) = line.get(project_name_key).and_then(|s| s.as_str()) else {
                    bail!("Unable to find `{project_name_key}` in invoice_line: {line:?}")
                };
                let service_name_key = "service_name";
                let Some(service_name) = line.get(service_name_key).and_then(|s| s.as_str()) else {
                    bail!("Unable to find `{service_name_key}` in invoice_line: {line:?}")
                };

                let timestamp_begin_key = "timestamp_begin";
                let Some(timestamp_begin) = line.get(timestamp_begin_key).and_then(|s| s.as_str())
                else {
                    bail!("Unable to find `{timestamp_begin_key}` in invoice_line: {line:?}")
                };
                let time_invoice_line_begins = DateTime::parse_from_rfc3339(timestamp_begin)?;

                let Some(line_total_local) = line.get("line_total_local").and_then(|s| s.as_str())
                else {
                    todo!();
                };
                let Some(local_currency) = line.get("local_currency").and_then(|s| s.as_str())
                else {
                    todo!();
                };

               topics =
                   get_aiven_kafka_topics(aiven_client, cfg, project_name, service_name).await?;


               tags.push(
                   get_aiven_service_tags(aiven_client, cfg, project_name, service_name).await?,
               );

                // this is only accidentally a datatype
                kafka_cost.push(KafkaLine {
                    timestamp_begin: timestamp_begin.into(),
                    service_name: service_name.into(),
                    project_name: project_name.into(),
                    line_total_local: line_total_local.into(),
                    local_currency: local_currency.into(),
                });
            }
        }
    }
    Ok((topics, tags, kafka_cost))
}

fn transform(
    topics: Vec<serde_json::Value>,
    tags: Vec<serde_json::Map<String, serde_json::Value>>,
    kafka_lines: Vec<KafkaLine>,
) {
    let total_kafka_cost = kafka_lines
        .iter() // string -> Result<(bd)>
        .flat_map(|l| BigDecimal::from_str_radix(&l.line_total_local, 10))
        .fold(BigDecimal::zero(), |acc, item| acc + item);

    let teams: HashSet<_>= topics
        .iter()
        .flat_map(|topic| topic.get("topic_name").and_then(|i| i.as_str()))
        .flat_map(|i| {
            i.splitn(1, ".").take(1)
        }).collect();

    let topic_size: HashSet<_>= topics
        .iter()
        .flat_map(|topic| topic.get("topic_name").and_then(|i| i.as_str()))
        .flat_map(|i| {
            i.splitn(1, ".").take(1)
        }).collect();


    let number_of_teams = topics
        .iter()
        .flat_map(|topic| topic.get("topic_name").and_then(|i| i.as_str()))
        .flat_map(|i| {
            i.splitn(1, ".").take(1)
        }).collect::<HashSet<_>>().len();


    let team_cost = BigDecimal::half(&total_kafka_cost) / BigDecimal::from_usize(number_of_teams).unwrap();

    // let disk_rate =
    // let weighted_team_cost = BigDecimal::half(&total_kafka_cost) * disk_rate

}

fn smear() {
    // let basis_cost = kafka_cluster_cost
                          // V teamcost

    // let teamcost =
           // V is invoice data
    0.5*basis_cost/(number_of_teams)
                                 // V topic
    //     + (0.5*basis_cost*(used_disk/topicsdataconsumption)
    //     + remote_storage_cost_for_team_topics
}

fn load() {}
