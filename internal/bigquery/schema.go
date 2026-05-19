package bigquery

type Line struct {
	BillingGroupId string `bigquery:"billing_group_id"`
	InvoiceId      string `bigquery:"invoice_id"`
	ProjectName    string `bigquery:"project_name"`
	Environment    string `bigquery:"environment"`
	Team           string `bigquery:"team"`
	Service        string `bigquery:"service"`
	ServiceName    string `bigquery:"service_name"`
	Tenant         string `bigquery:"tenant"`
	Status         string `bigquery:"status"`
	Cost           string `bigquery:"cost"`
	Currency       string `bigquery:"currency"`
	Date           string `bigquery:"date"`
	NumberOfDays   int    `bigquery:"number_of_days"`
}

type CurrencyRate struct {
	Date   string `bigquery:"date"`
	USDEUR string `bigquery:"usdeur"`
	USDNOK string `bigquery:"usdnok"`
}
