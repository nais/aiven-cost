use std::{collections::HashMap, io::IsTerminal};

use bigdecimal::{BigDecimal, FromPrimitive, Zero};
use chrono::{DateTime, Datelike, NaiveDate, Utc};
use color_eyre::eyre::{ContextCompat, Result, anyhow, bail};
use futures_util::future::try_join_all;
use gcloud_bigquery::{
    client::{Client, ClientConfig},
    http::{
        bigquery_table_client::BigqueryTableClient,
        job::query::QueryRequest,
        table::{Table, TableFieldSchema, TableFieldType, TableSchema},
        tabledata::insert_all::{InsertAllRequest, Row},
    },
    storage::row::Row as ReadRow,
};
use serde::Serialize;
use tracing::info;
use tracing_subscriber::{
    EnvFilter, Registry, prelude::__tracing_subscriber_SubscriberExt, util::SubscriberInitExt,
};

mod aiven;
use aiven::{AivenApiKafka, AivenApiKafkaInvoiceLine, AivenInvoice, KafkaInvoiceLineCostType};

pub fn init_tracing_subscriber() -> Result<()> {
    use tracing_subscriber::fmt as layer_fmt;

    let (plain_log_format, json_log_format) = match std::io::stdout().is_terminal() {
        true => (Some(layer_fmt::layer().compact()), None),
        false => (None, Some(layer_fmt::layer().json().flatten_event(true))),
    };

    Registry::default()
        .with(plain_log_format)
        .with(json_log_format)
        .with(EnvFilter::from_default_env())
        .try_init()?;

    Ok(())
}

const USER_AGENT: &str = "nais.io-kafka-cost";

fn client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .https_only(true)
        .user_agent(USER_AGENT)
//        .pool_max_idle_per_host(max)
        .build()
        .map_err(color_eyre::eyre::Error::msg)
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
            bigquery_dataset: "aiven_cost_regional".into(),
            bigquery_table: "kafka_cost".into(),
        }
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new();
    let aiven_client = client()?;

    let (config, _) = ClientConfig::new_with_auth().await?;
    let bigquery_client = Client::new(config).await?;
    let paid_invoices: Vec<BigQueryTableRowData> =
        get_rows_in_bigquery_table(&cfg, &bigquery_client)
            .await?
            .into_iter()
            .filter(|r| r.status == "paid")
            .collect();

    info!("Next we are finding the latest paid invoice line in BigQuery");
    let latest_date_string =
        paid_invoices
            .iter()
            .fold("2025-01", |current_oldest, current_invoice| {
                let current_invoice_date = &current_invoice.date;
                if **current_invoice_date > *current_oldest {
                    return current_invoice_date;
                }
                current_oldest
            });
    let date_of_latest_paid_invoice: DateTime<Utc> = DateTime::from_naive_utc_and_offset(
        NaiveDate::parse_from_str(&format!("{}-01", latest_date_string), "%Y-%m-%d")?.into(),
        Utc,
    );
    info!(
        "Latest paid invoice date in BigQuery: {}",
        date_of_latest_paid_invoice
    );

    let (kafka_base_cost_lines, kafka_base_tiered_storage_lines) =
        extract(&aiven_client, &cfg, &date_of_latest_paid_invoice).await?;
    let data = transform(&kafka_base_cost_lines, &kafka_base_tiered_storage_lines)?;

    load(&cfg, &bigquery_client, data).await?;

    info!("kafka-cost completed successfully");
    Ok(())
}

async fn get_rows_in_bigquery_table(
    cfg: &Cfg,
    bigquery_client: &Client,
) -> Result<Vec<BigQueryTableRowData>> {
    info!("Fetching rows from BigQuery table: {}", cfg.bigquery_table);
    let table_reference = gcloud_bigquery::http::table::TableReference {
        project_id: cfg.bigquery_project_id.clone(),
        dataset_id: cfg.bigquery_dataset.clone(),
        table_id: cfg.bigquery_table.clone(),
    };
    let mut reader = match bigquery_client
        .read_table::<ReadRow>(&table_reference, None)
        .await
    {
        Ok(r) => r,
        Err(_) => return Ok(Vec::new()),
    };
    let mut rows = Vec::new();
    while let Some(row) = reader.next().await? {
        let project_name = row.column::<String>(0)?;
        let environment = row.column::<String>(1)?;
        let team = row.column::<String>(2)?;
        let service = row.column::<String>(3)?;
        let status = row.column::<String>(4)?;
        let service_name = row.column::<String>(5)?;
        let tenant = row.column::<String>(6)?;
        let cost = row.column::<BigDecimal>(7)?;
        let date = row.column::<String>(8)?;
        rows.push(BigQueryTableRowData {
            project_name,
            environment,
            team,
            service,
            status,
            service_name,
            tenant,
            cost,
            date,
            ..Default::default()
        });
    }
    Ok(rows)
}

async fn extract(
    aiven_client: &reqwest::Client,
    cfg: &Cfg,
    date_of_latest_paid_invoice: &DateTime<Utc>,
) -> Result<(Vec<AivenApiKafkaInvoiceLine>, Vec<AivenApiKafkaInvoiceLine>)> {
    info!("Fetching invoices");
    let mut invoices: Vec<_> = AivenInvoice::from_aiven_api(aiven_client, cfg).await?;
    invoices.retain(|invoice| invoice.period_begin > *date_of_latest_paid_invoice);

    info!(
        "Found {} invoices not processed in BigQuery",
        invoices.len()
    );

    info!("Getting invoice lines for kakfa");
    let mut kafka_invoice_lines: Vec<AivenApiKafkaInvoiceLine> =
        try_join_all(invoices.iter().map(|invoice| {
            AivenApiKafkaInvoiceLine::from_aiven_api(aiven_client, cfg, &invoice.id, &invoice.state)
        }))
        .await?
        .into_iter()
        .flatten()
        .collect();

    let kafka_tiered_storage_cost_invoice_lines: Vec<_> = kafka_invoice_lines
        .iter()
        .filter(|&i| i.cost_type == KafkaInvoiceLineCostType::TieredStorage)
        .cloned()
        .collect();

    kafka_invoice_lines = try_join_all(
        kafka_invoice_lines
            .iter_mut()
            .filter(|i| i.cost_type == KafkaInvoiceLineCostType::Base)
            .map(|invoice_line| invoice_line.populate_with_tags_from_aiven_api(aiven_client, cfg)),
    )
    .await?
    .into_iter()
    .filter(|v| !v.kafka_instance.tenant.is_empty())
    .collect::<Vec<AivenApiKafkaInvoiceLine>>();

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

    info!("Fetching topics per kafka invoice line");
    // There's a bunch of suspicious try_join_alls in this codebase, they should probably be chunked
    // or limited concurrency rather than a herd of 1000 threads hitting aiven.
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

#[derive(Hash, Eq, PartialEq, Debug, Clone)]
struct TenantEnv {
    tenant: String,
    environment: String,
    project_name: String,
}

type TeamName = String;
struct KafkaInstance {
    service_name: String,
    base_cost: BigDecimal,
    aggregate_data_usage: DataUsage,
    invoice_state: String,
    teams: HashMap<TeamName, DataUsage>,
    year_month: String,
    days_in_month: u8,
}

#[derive(Eq, PartialEq, Debug, Default, Clone)]
struct DataUsage {
    base_size: u64,
    tiered_size: u64,
}

#[derive(Serialize, Debug, Default, PartialEq, Eq, Clone)]
struct BigQueryTableRowData {
    project_name: String,
    environment: String,
    team: String,
    service: String,
    status: String,
    service_name: String,
    tenant: String,
    #[serde(serialize_with = "big_decimal_truncat_serialization")]
    cost: BigDecimal,
    date: String,
    number_of_days: u8,
}

fn big_decimal_truncat_serialization<S>(
    value: &BigDecimal,
    serializer: S,
) -> Result<S::Ok, S::Error>
where
    S: serde::Serializer,
{
    serializer.serialize_str(&format!("{value:.2}"))
}

fn transform(
    kafka_base_cost_lines: &[AivenApiKafkaInvoiceLine],
    kafka_tiered_storage_cost_lines: &[AivenApiKafkaInvoiceLine],
) -> Result<Vec<BigQueryTableRowData>> {
    /// Sum up all topics' sizes per team
    fn aggregate_topic_usage_by_team(
        kafka_instance: &AivenApiKafka,
    ) -> Result<HashMap<TeamName, DataUsage>> {
        info!(
            "aggregating usage per team for {}",
            kafka_instance.service_name
        );
        kafka_instance
            .topics
            .iter()
            .map(|topic| {
                let Some(team_name) = topic.name.split('.').next() else {
                    bail!("Unable to find team name in topic name: '{}'", topic.name)
                };
                Ok((
                    team_name.to_owned(),
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
                ))
            })
            .collect()
    }

    // We start by making collection of all tenants>envs>instances>teams, and their usage of Kafka
    let mut tenant_envs: HashMap<TenantEnv, Vec<KafkaInstance>> = HashMap::new();
    for line in kafka_base_cost_lines {
        // Get existing TenantEnv's HashMap of kafka instances if it exists
        let tenant_key = TenantEnv {
            tenant: line.kafka_instance.tenant.clone(),
            environment: line.kafka_instance.environment.clone(),
            project_name: line.project_name.clone(),
        };
        let kafka_instances = tenant_envs.entry(tenant_key).or_default();
        let teams = aggregate_topic_usage_by_team(&line.kafka_instance)?;

        // Insert data into collector variable
        kafka_instances.push(KafkaInstance {
            service_name: line.service_name.clone(),
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
            invoice_state: line.invoice_state.clone(),
            base_cost: line.line_total_local.clone(),
            year_month: line.timestamp_begin.format("%Y-%m").to_string(),
            days_in_month: line.timestamp_begin.num_days_in_month(),
        });
    }

    // With the collection we can calcuate each teams usage of Kafka by their topics combined byte size
    let mut bigquery_data_rows = Vec::new();
    for (tenant_env, kafka_instances) in &tenant_envs {
        for instance in kafka_instances {
            let Some(num_teams) = BigDecimal::from_usize(instance.teams.len()) else {
                bail!("Unable to convert to BigDecimal: {}", instance.teams.len())
            };
            info!(
                "calculating kafka cost for {} teams for {}/{}/{}",
                instance.teams.len(),
                tenant_env.tenant,
                tenant_env.environment,
                instance.service_name
            );
            for (team_name, team_data_usage) in &instance.teams {
                let half_base_cost = instance.base_cost.clone() / 2;
                let team_divided_base_cost = &half_base_cost / num_teams.clone();

                let storage_weight =
                    team_data_usage.base_size / instance.aggregate_data_usage.base_size;
                let storage_weighted_storage_cost =
                    &team_divided_base_cost + (half_base_cost * storage_weight);

                bigquery_data_rows.push(BigQueryTableRowData {
                    project_name: tenant_env.project_name.clone(),
                    environment: tenant_env.environment.clone(),
                    team: team_name.clone(),
                    service: String::from("kafka-base"),
                    status: instance.invoice_state.to_string(),
                    service_name: instance.service_name.clone(),
                    tenant: tenant_env.tenant.clone(),
                    cost: team_divided_base_cost + storage_weighted_storage_cost,
                    date: instance.year_month.clone(),
                    number_of_days: instance.days_in_month,
                });
            }
        }
    }

    // Not every Kafka instance has tiered storage, so we iterate separately
    // for the teams using it, calculate based on their tiered storage size
    for line in kafka_tiered_storage_cost_lines {
        if line.kafka_instance.topics.is_empty() {
            continue;
        }
        let project_name = &line.project_name;
        let env = &line.kafka_instance.environment;
        let tenant = &line.kafka_instance.tenant;
        let instances = &tenant_envs[&TenantEnv {
            tenant: tenant.to_owned(),
            environment: env.to_owned(),
            project_name: project_name.to_owned(),
        }];

        let service_name = &line.service_name;
        let date_as_string = line.timestamp_begin.format("%Y-%m").to_string();
        let instance = &instances
            .iter()
            .find(|i| i.service_name == *service_name && i.year_month == date_as_string)
            .wrap_err(format!("No instance found for service: {}", service_name))?;

        let total_tiered_storage = instance.aggregate_data_usage.tiered_size;
        for (name, usage) in &instance.teams {
            if usage.tiered_size.is_zero() {
                continue;
            }
            info!("adding tiered storage cost for {}", name);
            bigquery_data_rows.push(BigQueryTableRowData {
                project_name: project_name.to_owned(),
                environment: env.to_owned(),
                team: name.to_owned(),
                service: String::from("kafka-tiered"),
                status: instance.invoice_state.to_string(),
                service_name: service_name.to_owned(),
                tenant: tenant.to_owned(),
                cost: &line.line_total_local * (usage.tiered_size / total_tiered_storage),
                date: date_as_string.clone(),
                number_of_days: line.timestamp_begin.num_days_in_month(),
            });
        }
    }

    let should_not_contain = [
        // Kafka streams join metadata
        "JOINTHIS",
        "JOINOTHER",
    ];
    let should_not_start_with = [
        "__", // kafka connect meta topic, __connect_configs, __connect_offsets, __connect_status etc
    ];
    let should_not_end_with = [
        // Kafka streams meta/internals
        "-repartition",
        "-changelog",
    ];

    // There are topic names this program did not correctly attribute to teams.
    // Here's where we filter these topics (now team names at this stage in the program) out.
    let cleaned_topics: Vec<_> = bigquery_data_rows
        .into_iter()
        .filter(|r| !should_not_contain.iter().any(|c| r.team.contains(c)))
        .filter(|r| !should_not_start_with.iter().any(|c| r.team.starts_with(c)))
        .filter(|r| !should_not_end_with.iter().any(|c| r.team.ends_with(c)))
        .collect();

    Ok(cleaned_topics)
}

async fn load(cfg: &Cfg, client: &Client, rows: Vec<BigQueryTableRowData>) -> Result<()> {
    let actual_rows: Vec<Row<BigQueryTableRowData>> = rows
        .into_iter()
        .map(|r| Row {
            insert_id: None,
            json: r,
        })
        .collect();

    // This is backwards for reasons, if we fail at getting the table _for any_ reason we just try to create it.
    if (client
        .table()
        .get(
            &cfg.bigquery_project_id,
            &cfg.bigquery_dataset,
            &cfg.bigquery_table,
        )
        .await)
        .is_err()
    {
        info!("table doesn't exist, creating: {}", cfg.bigquery_table);
        create_table(cfg, client.table()).await?;
    }

    info!("deleting rows with status not in 'paid'");
    let query_response = client
        .job()
        .query(
            &cfg.bigquery_project_id,
            &QueryRequest {
                query: format!(
                    "DELETE FROM `{}.{}.{}` WHERE status NOT IN ('paid')",
                    &cfg.bigquery_project_id, &cfg.bigquery_dataset, &cfg.bigquery_table
                ),
                ..Default::default()
            },
        )
        .await?;
    if let Some(rs) = query_response.total_rows {
        info!("Number of rows deleted: {}", rs);
    }

    info!(
        "inserting {} rows into table: {}",
        actual_rows.len(),
        cfg.bigquery_table
    );
    let response = client
        .tabledata()
        .insert(
            &cfg.bigquery_project_id,
            &cfg.bigquery_dataset,
            &cfg.bigquery_table,
            &InsertAllRequest {
                rows: actual_rows,
                ..Default::default()
            },
        )
        .await?;
    response
        .insert_errors
        .into_iter()
        .for_each(|e| info!("Error inserting row: {:?}", e));

    Ok(())
}

fn string_field(name: &str) -> TableFieldSchema {
    TableFieldSchema {
        name: name.to_owned(),
        data_type: TableFieldType::String,
        ..Default::default()
    }
}
async fn create_table(cfg: &Cfg, client: &BigqueryTableClient) -> Result<Table> {
    info!("creating table");
    let table = Table {
        table_reference: gcloud_bigquery::http::table::TableReference {
            project_id: cfg.bigquery_project_id.clone(),
            dataset_id: cfg.bigquery_dataset.clone(),
            table_id: cfg.bigquery_table.clone(),
        },
        schema: Some(TableSchema {
            fields: vec![
                string_field("project_name"),
                string_field("environment"),
                string_field("team"),
                string_field("service"),
                string_field("status"),
                string_field("service_name"),
                string_field("tenant"),
                TableFieldSchema {
                    name: "cost".to_owned(),
                    data_type: TableFieldType::Bignumeric,
                    ..Default::default()
                },
                string_field("date"),
                TableFieldSchema {
                    name: "number_of_days".to_owned(),
                    data_type: TableFieldType::Integer,
                    ..Default::default()
                },
            ],
        }),
        ..Default::default()
    };

    client
        .create(&table)
        .await
        .map_err(|e| anyhow!("Failed to create table: {}", e))
}
