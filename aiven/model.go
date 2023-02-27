package aiven

import (
	"time"
)

type Tags struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment"`
	Team        string `json:"team"`
}

type InvoiceLine struct {
	TimestampBegin time.Time `json:"timestamp_begin"`
	TimestampEnd   time.Time `json:"timestamp_end"`
	Cost           string    `json:"line_total_local"`
	Currency       string    `json:"local_currency"`
	ServiceType    string    `json:"service_type"`
	ServiceName    string    `json:"service_name"`
	ProjectName    string    `json:"project_name"`
	LineType       string    `json:"line_type"`
}

type Invoice struct {
	InvoiceId   string `json:"invoice_number"`
	TotalIncVat string `json:"total_inc_vat"`
	Status      string `json:"invoice_state"`
}

type BillingGroups []BillingGroup

type BillingGroup struct {
	AccountName      string `json:"account_name"`
	BillingGroupId   string `json:"billing_group_id"`
	BillingGroupName string `json:"billing_group_name"`
	BillingCurrency  string `json:"billing_currency"`
}

/*func (i *InvoiceLine) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"timestamp_begin": i.TimestampBegin,
		"timestamp_end":   i.TimestampEnd,
		"cost":            i.Cost,
		"service_name":    i.ServiceName,
		"currency":        i.Currency,
		"service_type":    i.ServiceType,
		"project_name":    i.ProjectName,
		"line_type":       i.LineType,
		//"status":          i.Status,
		//"invoice_id":      i.InvoiceId,
	}, "", nil
}*/
