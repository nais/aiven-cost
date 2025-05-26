use std::collections::{HashMap, HashSet};

use anyhow::Result;
use bigdecimal::{BigDecimal, Zero};
use futures_util::future::try_join_all;
use gcp_bigquery_client::{
    Client, model::table_data_insert_all_request::TableDataInsertAllRequest,
};
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
    let data = transform(&kafka_base_cost_lines, &kafka_base_tiered_storage_lines);

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

struct BigQueryTableRow {}

fn transform(
    kafka_base_cost_lines: &[AivenApiKafkaInvoiceLine],
    kafka_tiered_storage_cost_lines: &[AivenApiKafkaInvoiceLine],
) -> Result<Vec<BigQueryTableRow>> {
    let total_kafka_cost = kafka_base_cost_lines
        .iter() // string -> Result<(bd)>
        .map(|l| &l.line_total_local)
        .fold(BigDecimal::zero(), |acc, item| acc + item);

    let teams_per_tenant: HashMap<String, HashSet<_>> = kafka_base_cost_lines
        .iter()
        .map(|kafka_line_item| {
            (
                kafka_line_item.kafka_instance.tenant.clone(),
                kafka_line_item
                    .kafka_instance
                    .topics
                    .iter()
                    .flat_map(|t| t.name.splitn(2, '.').nth(0))
                    .collect(),
            )
        })
        .collect();

    #[derive(Hash, Eq, PartialEq, Debug, Clone)]
    struct Key {
        tenant: String,
        environment: String,
        kafka_instance: String,
        team: String,
    }

    struct TenantEnv {
        tenant: String,
        environment: String,
        kafkas: HashMap<String, KafkaInstance>,
    }

    type TeamName = String;
    struct KafkaInstance {
        data_usage: DataUsage,
        teams: HashMap<TeamName, DataUsage>,
    }

    #[derive(Eq, PartialEq, Debug, Clone)]
    struct DataUsage {
        base_size: u64,
        tiered_size: u64,
    }

    let tenants: Vec<TenantEnv> = kafka_base_cost_lines
        .iter()
        .map(|h| {
            let mut kafkas = HashMap::new();
            kafkas.insert(
                h.kafka_instance.service_name.clone(),
                KafkaInstance {
                    data_usage: DataUsage {
                        base_size: 0,
                        tiered_size: 0,
                    },
                    teams: h
                        .kafka_instance
                        .topics
                        .iter()
                        .map(|topic| {
                            (
                                topic.name.splitn(2, '.').nth(0).unwrap().to_owned(),
                                topic
                                    .partitions
                                    .iter()
                                    .map(|partition| {
                                        (partition.size, partition.remote_size.unwrap_or(0))
                                    })
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
                        .collect::<HashMap<String, DataUsage>>(),
                },
            );

            TenantEnv {
                tenant: h.kafka_instance.tenant.clone(),
                environment: h.kafka_instance.environment.clone(),
                kafkas,
            }
        })
        .collect();

    let mut old_tenants: HashMap<Key, DataUsage> = HashMap::new();
    kafka_base_cost_lines
        .iter()
        .map(|line| {
            (
                line.kafka_instance.tenant.clone(),
                line.kafka_instance.environment.clone(),
                line.kafka_instance.service_name.clone(),
                line.kafka_instance.topics.clone(),
            )
        })
        .for_each(|(tenant, environment, kafka_instance, topics)| {
            topics
                .iter()
                .map(|topic| {
                    (
                        topic.name.splitn(2, '.').nth(0).unwrap(),
                        topic
                            .partitions
                            .iter()
                            .map(|partition| (partition.size, partition.remote_size.unwrap_or(0)))
                            .collect::<Vec<(u64, u64)>>(),
                    )
                })
                .for_each(|(team, values)| {
                    let base_size: u64 = values.iter().map(|(base_size, _)| base_size).sum();
                    let tiered_size: u64 = values.iter().map(|(_, tiered_size)| tiered_size).sum();

                    let sizes = old_tenants
                        .entry(Key {
                            tenant: tenant.clone(),
                            environment: environment.clone(),
                            kafka_instance: kafka_instance.clone(),
                            team: team.to_string(),
                        })
                        .or_insert_with(|| DataUsage {
                            base_size: 0,
                            tiered_size: 0,
                        });
                    sizes.base_size += base_size;
                    sizes.tiered_size += tiered_size;
                });
        });

    dbg!(old_tenants);

    for (tenant, teams) in teams_per_tenant {
        info!(
            "Tenant '{tenant}' has {} unique team names across environments",
            teams.len()
        );
    }

    // BigQuery(aiven_cost_regional.cost_items): billing_group_id, invoice_id, project_name, environment, team, service, service_name, tenant, status, cost, currency, date, number_of_days
    // BigQuery(aiven_cost_regional.cost_team): billing_group_id, invoice_id, project_name, environment, team, service(kafka_team), service_name(kafka-instance), tenant, status, cost, cost_type(base|tiered), currency, date, number_of_days

    // hash(key(tenant, env), value({}instance(key(instance_name), value([]teams(base_size, tiered_size)))))

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

    // let topic_size: HashSet<_> = topics
    //     .iter()
    //     .filter_map(|topic| topic.get("topic_name").and_then(|i| i.as_str()))
    //     .flat_map(|i| i.splitn(2, '.').take(1))
    //     .collect();

    // let number_of_teams = topics
    //     .iter()
    //     .filter_map(|topic| topic.get("topic_name").and_then(|i| i.as_str()))
    //     .flat_map(|i| i.splitn(2, '.').take(1))
    //     .collect::<HashSet<_>>()
    //     .len();

    // let team_cost =
    //     BigDecimal::half(&total_kafka_cost) / BigDecimal::from_usize(number_of_teams).unwrap();

    // let disk_rate =
    // let weighted_team_cost = BigDecimal::half(&total_kafka_cost) * disk_rate
    todo!()
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

fn load() {}
