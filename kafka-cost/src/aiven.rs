use anyhow::{bail, Result};
use bigdecimal::{BigDecimal, Num};

#[derive(Debug, PartialEq, Eq)]
pub enum AivenInvoiceState {
    Paid,
    Mailed,
    Estimate,
}

impl AivenInvoiceState {
    pub fn from_json_obj(obj: &serde_json::Value) -> Result<Self> {
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
pub struct AivenInvoice {
    pub id: String,
    pub total_inc_vat: String,
    pub state: AivenInvoiceState,
}

impl AivenInvoice {
    pub fn from_json_obj(obj: &serde_json::Value) -> Result<Self> {
        let total_inc_vat_key = "total_inc_vat";
        let Some(total_inc_vat) = obj.get(total_inc_vat_key).and_then(|s| s.as_str()) else {
            bail!("Unable to find `{total_inc_vat_key}` in: {obj:?}")
        };
        // TODO: Verify this is correct, and we're not supposed to turn it into float first or something
        // let Ok(total_inc_vat) = BigDecimal::from_str_radix(total_inc_vat_json, 10) else {
        //     bail!("Unable to parse as bigdecimal: '{total_inc_vat_json}'")
        // };

        let invoice_number_key = "invoice_number";
        let Some(id) = obj.get(invoice_number_key).and_then(|s| s.as_str()) else {
            bail!("Unable to parse/find invoice's '{invoice_number_key}': {obj:?}")
        };

        let state_key = "state";
        let Some(state_json) = obj.get(state_key) else {
            bail!("Unable to find invoice's '{state_key}': {obj:?}")
        };
        let state = AivenInvoiceState::from_json_obj(state_json)?;

        Ok(Self {
            id: id.to_string(),
            total_inc_vat: total_inc_vat.to_string(),
            state,
        })
    }

    pub fn from_json_list(list: &[serde_json::Value]) -> Result<Vec<Self>> {
        Ok(list.iter().flat_map(Self::from_json_obj).collect())
    }
}

pub struct KafkaLine {
    pub timestamp_begin: String,
    pub service_name: String,
    pub project_name: String,
    pub line_total_local: String,
    pub local_currency: String,
}

pub struct AivenInvoiceLine {}
