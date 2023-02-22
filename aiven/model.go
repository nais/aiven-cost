package aiven

import "time"

type Tags struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment"`
	Team        string `json:"team"`
}

type InvoiceDetail struct {
	TimestampBegin time.Time `json:"timestamp_begin"`
	TimestampEnd   time.Time `json:"timestamp_end"`
	Cost           string    `json:"line_total_local"`
	ServiceName    string    `json:"service_name"`
	Currency       string    `json:"local_currency"`
	ServiceType    string    `json:"service_type"`
	ProjectName    string    `json:"project_name"`
	LineType       string    `json:"line_type"`
	Description    string    `json:"description"`
}

type Invoice struct {
	InvoiceId   string `json:"invoice_number"`
	TotalIncVat string `json:"total_inc_vat"`
}

type BillingGroups []BillingGroup

type BillingGroup struct {
	AccountName      string `json:"account_name"`
	BillingGroupId   string `json:"billing_group_id"`
	BillingGroupName string `json:"billing_group_name"`
	BillingCurrency  string `json:"billing_currency"`
}
