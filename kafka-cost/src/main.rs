use anyhow::Result;
use tracing::info;
use std::collections::HashMap;
use serde_json::{json, Value};
use reqwest::{header, ClientBuilder};

pub fn init_tracing_subscriber() -> Result<()> {
    tracing_subscriber::fmt()
        .init();
    Ok(())
}


fn client(token: &str) -> Result<reqwest::Client>  {
    reqwest::Client::builder()
    .https_only(true).user_agent("nais-kafka-cost")
        .build().map_err(anyhow::Error::msg)


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


#[tokio::main]
async fn main() -> Result<()>{
    init_tracing_subscriber()?;
    info!("started kafka-cost");
    let cfg = Cfg::new()?;
    let aiven_client = client(&cfg.aiven_api_token)?;

    let res = aiven_client.get(&format!("https://api.aiven.io/v1/billing-group/{}/invoice", cfg.billing_group_id)).bearer_auth(cfg.aiven_api_token).send().await?;
    let mut lookup: HashMap<String, Value> = serde_json::from_str(&res.text().await?).unwrap();
    dbg!(lookup);
    Ok(())
}
