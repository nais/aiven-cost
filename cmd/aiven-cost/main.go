package main

import (
	"context"
	"flag"
	"os"

	"github.com/nais/aiven-cost/aiven"
	"github.com/nais/aiven-cost/bigquery"
	"github.com/nais/aiven-cost/log"
)

var cfg = struct {
	APIHost        string
	LogLevel       string
	AivenToken     string
	CostItemsTable string
	CurrencyTable  string
}{
	APIHost:    "api.aiven.io",
	LogLevel:   "info",
	AivenToken: "",
}

func main() {
	ctx := context.Background()

	flag.StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")
	flag.StringVar(&cfg.CostItemsTable, "cost-table", "cost_items", "Name of cost table in BigQuery")
	flag.StringVar(&cfg.CurrencyTable, "currency-table", "currency_rates", "Name of currency table in BigQuery")
	flag.Parse()
	f, err := os.Create("aiven-cost.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	aivenClient := aiven.New(cfg.APIHost, cfg.AivenToken)

	/*	currencyClient := currency.New()
		currency, err := currencyClient.GetRates(ctx)
		if err != nil {
			log.Errorf(err, "failed to get currency rates")
		}
	*/
	bqClient := bigquery.New(ctx)
	log.Infof("create bigquery table if not exists")
	err = bqClient.CreateIfNotExists(ctx, bigquery.Line{}, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to create cost table")
	}
	log.Infof("delete unpaid cost lines from bigquery")
	err = bqClient.DeleteUnpaid(ctx, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to delete unpaid cost lines")
		panic(err)
	}
	log.Infof("fetch cost item id and status from bigquery")
	bqInvoices, err := bqClient.FetchCostItemIDAndStatus(ctx, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to fetch cost item id and status")
		panic(err)
	}
	// fetch aiven invoice ids
	log.Infof("fetch aiven invoice ids")
	aivenInvoiceIDs, err := aivenClient.GetInvoiceIDs(ctx)
	if err != nil {
		log.Errorf(err, "failed to get invoice ids")
		panic(err)
	}
	log.Infof("filter out payed invoices")
	// filter out payed invoices
	unprocessedInvoices := filterPayedInvoices(aivenInvoiceIDs, bqInvoices)

	log.Infof("fetch invoice details from aiven and insert into bigquery for %d invoices of a total of %d", len(unprocessedInvoices), len(aivenInvoiceIDs))
	// fetch invoice details for each invoice and map to bigquery schema
	for invoiceID, billingGroup := range unprocessedInvoices {
		invoice, err := aivenClient.GetInvoice(ctx, billingGroup, invoiceID)
		if err != nil {
			log.Errorf(err, "failed to get invoice for invoice %s", invoiceID)
			panic(err)
		}

		invoiceLines, err := aivenClient.GetInvoiceLines(ctx, billingGroup, invoice)
		if err != nil {
			log.Errorf(err, "failed to get invoice details for invoice %s", invoiceID)
			panic(err)
		}

		// insert invoice into bigquery
		err = bqClient.InsertCostItems(ctx, invoiceLines, cfg.CostItemsTable)
		if err != nil {
			log.Errorf(err, "failed to insert cost item into bigquery")
			panic(err)
		}
		/*for _, line := range invoiceLines {
			err := bqClient.InsertCostItems(ctx, line, cfg.CostItemsTable)
			retries := 0
			for err != nil {
				if retries > 5 {
					log.Errorf(err, "failed to insert cost item into bigquery")
					panic(err)
				}
				time.Sleep(time.Duration(retries) * time.Second * 5)
				retries++
				log.Errorf(err, "failed to insert cost item into bigquery. retrying...")
				err = bqClient.InsertCostItems(ctx, line, cfg.CostItemsTable)
			}
		}*/
	}
}

func filterPayedInvoices(aivenInvoiceIDs, bqInvoices map[string]string) map[string]string {
	unprocessedInvoices := make(map[string]string)
	for invoiceID, billingGroup := range aivenInvoiceIDs {
		if _, ok := bqInvoices[invoiceID]; ok && bqInvoices[invoiceID] == "paid" {
			continue
		}
		unprocessedInvoices[invoiceID] = billingGroup
	}
	return unprocessedInvoices
}
