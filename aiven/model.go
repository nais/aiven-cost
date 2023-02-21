package aiven

import "time"

type Projects []Project

type Project struct {
	Name         string `json:"project_name"`
	BillingGroup string `json:"billing_group_id"`
}

type Services []Service

type Tags struct {
	Tenant      string `json:"tenant"`
	Environment string `json:"environment"`
	Team        string `json:"team"`
}

type Service struct {
	Tags        Tags   `json:"tags"`
	ServiceName string `json:"service_name"`
	ServiceType string `json:"service_type"`
}

type InvoiceDetail struct {
	TimestampBegin time.Time `json:"timestamp_begin"`
	TimestampEnd   time.Time `json:"timestamp_end"`
	Cost           string    `json:"line_total_local"`
	ServiceName    string    `json:"service_name"`
	Currency       string    `json:"local_currency"`
	ServiceType    string    `json:"service_type"`
	ProjectName    string    `json:"project_name"`
}

type Invoice struct {
	InvoiceId   string `json:"invoice_number"`
	TotalIncVat string `json:"total_inc_vat"`
}
