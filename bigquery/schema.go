package bigquery

import "cloud.google.com/go/bigquery"

type Line struct {
	InvoiceId    string `bigquery:"invoice_id"`
	ProjectName  string `bigquery:"project_name"`
	Environment  string `bigquery:"environment"`
	Team         string `bigquery:"team"`
	Service      string `bigquery:"service"`
	ServiceName  string `bigquery:"service_name"`
	Tenant       string `bigquery:"tenant"`
	Status       string `bigquery:"status"`
	Cost         string `bigquery:"cost"`
	Currency     string `bigquery:"currency"`
	Date         string `bigquery:"date"`
	NumberOfDays int    `bigquery:"number_of_days"`
}

func (l *Line) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"invoice_id":     l.InvoiceId,
		"project_name":   l.ProjectName,
		"environment":    l.Environment,
		"team":           l.Team,
		"service":        l.Service,
		"service_name":   l.ServiceName,
		"tenant":         l.Tenant,
		"status":         l.Status,
		"cost":           l.Cost,
		"currency":       l.Currency,
		"date":           l.Date,
		"number_of_days": l.NumberOfDays,
	}, "", nil
}
