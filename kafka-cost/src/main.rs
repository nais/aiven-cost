use std::collections::HashMap;

use anyhow::{Result, bail};
use bigdecimal::{BigDecimal, FromPrimitive, Zero};
use chrono::{DateTime, Utc};
use futures_util::future::try_join_all;
use gcp_bigquery_client::{
    Client, model::table_data_insert_all_request::TableDataInsertAllRequest,
};
use serde::Serialize;
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
    fn new() -> Self {
        Self {
            // Aiven stuff
            aiven_api_token: std::env::var("AIVEN_API_TOKEN").expect("api token"),
            billing_group_id: "7d14362d-1e2a-4864-b408-1cc631bc4fab".into(),
            // BQ stuff
            bigquery_project_id: "nais-io".into(),
            bigquery_dataset: "kafka_team_cost".into(),
            bigquery_table: "kafka_team_cost".into(),
        }
    }
}

mod aiven;
use aiven::{AivenApiKafkaInvoiceLine, AivenInvoice, AivenInvoiceState, KafkaInvoiceLineCostType};

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new();
    let aiven_client = client()?;
    let bigquery_client = Client::from_application_default_credentials().await?;

    let (kafka_base_cost_lines, kafka_base_tiered_storage_lines) =
        extract(&aiven_client, &cfg).await?;
    dbg!(&kafka_base_cost_lines.len());
    // dbg!(&kafka_base_cost_lines[0]);
    // todo!();
    let data = transform(
        &cfg.billing_group_id,
        &kafka_base_cost_lines,
        &kafka_base_tiered_storage_lines,
    )?;

    load(&bigquery_client, &data)?;

    Ok(())
}

async fn extract(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
) -> Result<(Vec<AivenApiKafkaInvoiceLine>, Vec<AivenApiKafkaInvoiceLine>)> {
    let invoices: Vec<_> = AivenInvoice::from_aiven_api(aiven_client, cfg).await?;
    let unpaid_invoices: Vec<_> = invoices
        .iter()
        // .filter(|i| i.state != AivenInvoiceState::Paid) // The below line avoids duplicate fetching of expensive Aiven API calls
        .filter(|i| i.state == AivenInvoiceState::Estimate)
        .collect();
    info!(
        "Out of {} invoice(s) from Aiven, {} is/are unpaid/estimate(s)",
        invoices.len(),
        unpaid_invoices.len(),
    );
    // dbg!(&unpaid_invoices);

    let mut kafka_invoice_lines: Vec<_> =
        try_join_all(unpaid_invoices.iter().map(|invoice| {
            AivenApiKafkaInvoiceLine::from_aiven_api(aiven_client, cfg, &invoice.id)
        }))
        .await?
        .into_iter()
        .flatten()
        .collect();
    // dbg!(&kafka_invoice_lines.len());
    let kafka_tiered_storage_cost_invoice_lines: Vec<_> = kafka_invoice_lines
        .iter()
        .filter(|&i| i.cost_type == KafkaInvoiceLineCostType::TieredStorage)
        .cloned()
        .collect();
    // dbg!(&kafka_tiered_storage_cost_invoice_lines.len());
    kafka_invoice_lines = try_join_all(
        kafka_invoice_lines
            .iter_mut()
            .filter(|i| i.cost_type == KafkaInvoiceLineCostType::Base)
            .map(|kafka_instance| {
                kafka_instance.populate_with_tags_from_aiven_api(aiven_client, cfg)
            }),
    )
    .await?;
    dbg!(&kafka_invoice_lines.len());
    assert!(
        &kafka_invoice_lines.iter().all(
            |k| !k.kafka_instance.tenant.is_empty() && !k.kafka_instance.environment.is_empty()
        ),
        "Missing environment & tenant out of the`AivenTags` we set over at Aiven",
    );
    assert!(
        &kafka_tiered_storage_cost_invoice_lines.iter().all(|k1| {
            kafka_invoice_lines
                .iter()
                .any(|k2| k2.service_name == k1.service_name && k1.project_name == k2.project_name)
        }),
        "Unable to find kafka instance tiered storage belongs to",
    );
    dbg!(&kafka_invoice_lines.len());
    dbg!(&kafka_invoice_lines[0]);

    Ok((
        try_join_all(
            kafka_invoice_lines
                .clone()
                .into_iter()
                .map(|kafka_instance| {
                    kafka_instance.populate_with_topics_from_aiven_api(aiven_client, cfg)
                }),
        )
        .await?,
        kafka_tiered_storage_cost_invoice_lines,
    ))
}

#[derive(Serialize)]
struct BigQueryTableRowData {
    billing_group_id: String,
    invoice_id: String,
    project_name: String,
    environment: String,
    team: String,
    service: String,
    service_name: String,
    tenant: String,
    status: String,
    cost: BigDecimal,
    currency: String,
    date: DateTime<Utc>,
    number_of_days: u8,
}

fn transform(
    billing_group_id: &str,
    kafka_base_cost_lines: &[AivenApiKafkaInvoiceLine],
    kafka_tiered_storage_cost_lines: &[AivenApiKafkaInvoiceLine],
) -> Result<TableDataInsertAllRequest> {
    #[derive(Hash, Eq, PartialEq, Debug, Clone)]
    struct TenantEnv {
        tenant: String,
        environment: String,
        project_name: String,
    }

    type TeamName = String;
    type KafkaInstanceName = String;
    struct KafkaInstance {
        base_cost: BigDecimal,
        tiered_cost: BigDecimal,
        aggregate_data_usage: DataUsage,
        teams: HashMap<TeamName, DataUsage>,
        currency: String,
        invoice_id: String,
    }

    #[derive(Eq, PartialEq, Debug, Default, Clone)]
    struct DataUsage {
        base_size: u64,
        tiered_size: u64,
    }
    // let foo: HashMap<TenantEnv, HashMap<KafkaInstanceName, KafkaInstance>> = HashMap::new();
    let mut tenant_envs: HashMap<TenantEnv, HashMap<KafkaInstanceName, KafkaInstance>> =
        HashMap::new();
    for line in kafka_base_cost_lines {
        // Get existing TenantEnv's HashMap of kafka instances if it exists
        let tenant_key = TenantEnv {
            tenant: line.kafka_instance.tenant.clone(),
            environment: line.kafka_instance.environment.clone(),
            project_name: line.project_name.clone(),
        };
        let kafka_instances = tenant_envs
            .entry(tenant_key)
            .or_insert_with(|| HashMap::new());

        // Sum up all topics' sizes per team
        let teams = line
            .kafka_instance
            .topics
            .iter()
            .map(|topic| {
                (
                    topic.name.splitn(2, '.').nth(0).unwrap().to_owned(),
                    topic
                        .partitions
                        .iter()
                        .map(|partition| (partition.size, partition.remote_size.unwrap_or(0)))
                        .fold(
                            DataUsage {
                                base_size: 0,
                                tiered_size: 0,
                            },
                            |mut acc, (base, tiered)| {
                                acc.base_size += base;
                                acc.tiered_size += tiered;
                                acc
                            },
                        ),
                )
            })
            .collect::<HashMap<String, DataUsage>>();

        // Insert data into collector variable
        kafka_instances.insert(
            line.service_name.clone(),
            KafkaInstance {
                invoice_id: line.invoice_id.clone(),

                aggregate_data_usage: teams.iter().fold(
                    DataUsage {
                        base_size: 0,
                        tiered_size: 0,
                    },
                    |mut acc, (_, du)| {
                        acc.base_size += du.base_size;
                        acc.tiered_size += du.tiered_size;
                        acc
                    },
                ),
                teams,
                base_cost: line.line_total_local.clone(),
                currency: line.local_currency.clone(),
                tiered_cost: BigDecimal::zero(),
            },
        );
    }

    let mut bigquery_data_rows = TableDataInsertAllRequest::new();
    for (tenant_env, kafka_instances) in tenant_envs {
        for (kafka_name, instance) in kafka_instances {
            let Some(num_teams) = BigDecimal::from_usize(instance.teams.len()) else {
                bail!("Unable to convert to BigDecimal: {}", instance.teams.len())
            };
            let half_base_cost = instance.base_cost / 2;
            let team_divided_base_cost = &half_base_cost / num_teams;
            for (team_name, team_data_usage) in instance.teams {
                // bqlines = []
                //
                // for tenants
                //   let cost
                //
                //   for env
                //    let env_cost
                //
                //    for kafka
                //      let kafka_cost
                //      let base_size = iter
                //      let tiered_size = iter
                //
                //      let base_cost_kroner = (kafka_cost/2)/#kafka.teams
                //      let base_storage_kroner = (kafka_cost/2)/base_size
                //
                //      for teams
                //        team_kost = base_cost_kroner + (base_storage_kroner * team.base_size)
                //        // TODO: tiered
                //
                //        bqlines.insert(tenant, env, kafka, team, team_kost)
                bigquery_data_rows.add_row(
                    None,
                    BigQueryTableRowData {
                        billing_group_id: billing_group_id.to_string(),
                        invoice_id: instance.invoice_id,
                        project_name: tenant_env.project_name,
                        environment: tenant_env.environment,
                        team: team_name,
                        service: String::from("kafka-base"),
                        service_name: kafka_name.to_string(),
                        tenant: tenant_env.tenant.to_string(),
                        cost: team_divided_base_cost
                            + (half_base_cost * team_data_usage.base_size
                                / instance.aggregate_data_usage.base_size),
                        currency: instance.currency,
                        status: todo!(),
                        date: todo!(),
                        number_of_days: todo!(),
                    },
                )?;
            }
        }
    }

    // TODO: Iterate through and add tiered storage costs to bigquery_data_rows

    Ok(bigquery_data_rows)
}

fn smear() {
    // let basis_cost = kafka_cluster_cost
    // V teamcost

    // let teamcost =
    // V is invoice data
    // 0.5 * basis_cost / (number_of_teams)
    // V topic
    //     + (0.5*basis_cost*(used_disk/topicsdataconsumption)
    //     + remote_storage_cost_for_team_topics
}

fn load(client: &Client, rows: &TableDataInsertAllRequest) -> Result<()> {
    todo!()
}
