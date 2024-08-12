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
	Status      string `json:"state"`
}
