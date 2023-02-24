package bigquery

type Line struct {
	InvoiceId   string `bigquery:"invoice_id"`
	Environment string `bigquery:"environment"`
	Team        string `bigquery:"team"`
	Service     string `bigquery:"service"`
	Tenant      string `bigquery:"tenant"`
	Status      string `bigquery:"status"`
	Cost        string `bigquery:"cost"`
	Date        string `bigquery:"date"`
}
