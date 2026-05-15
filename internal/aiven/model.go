package aiven

type billingTags struct {
	Tenant      string `json:"billing:tenant"`
	Environment string `json:"billing:environment"`
}

type Invoice struct {
	InvoiceId   string `json:"invoice_number"`
	TotalIncVat string `json:"total_inc_vat"`
	Status      string `json:"state"`
}

type orgInvoiceLinesResponse struct {
	Lines []orgInvoiceLine `json:"lines"`
}

type orgInvoiceLine struct {
	BeginTime   string      `json:"begin_time"`
	Cloud       string      `json:"cloud"`
	Currency    string      `json:"currency"`
	Description string      `json:"description"`
	EndTime     string      `json:"end_time"`
	LineType    string      `json:"line_type"`
	Plan        string      `json:"plan"`
	ProjectID   string      `json:"project_id"`
	ServiceID   string      `json:"service_id"`
	ServiceType string      `json:"service_type"`
	Tags        billingTags `json:"tags"`
	Total       string      `json:"total"`
	TotalUSD    string      `json:"total_usd"`
}
