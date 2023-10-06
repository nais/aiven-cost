package main

import (
	"context"
	"flag"
	"os"

	"github.com/nais/aiven-cost/internal/aiven"
	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/nais/aiven-cost/internal/config"
	"github.com/nais/aiven-cost/internal/log"
)

func main() {
	ctx := context.Background()

	cfg := config.New()

	flag.StringVar(&cfg.AivenAPI, "api-host", cfg.AivenAPI, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")
	flag.StringVar(&cfg.CurrencyToken, "currency-token", os.Getenv("CURRENCY_TOKEN"), "Currency API token")
	flag.Parse()

	aivenClient := aiven.New(cfg.AivenAPI, cfg.AivenToken)

	bqClient := bigquery.New(ctx, cfg, "europe-north1")

	log.Infof("create bigquery table if not exists")
	err := bqClient.CreateIfNotExists(ctx, bigquery.Line{}, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to create cost table")
	}
	log.Infof("delete unpaid cost lines from bigquery")
	err = bqClient.DeleteUnpaid(ctx)
	if err != nil {
		log.Errorf(err, "failed to delete unpaid cost lines")
		os.Exit(1)
	}
	log.Infof("fetch cost item id and status from bigquery")
	bqInvoices, err := bqClient.FetchCostItemIDAndStatus(ctx)
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

		err = bqClient.InsertCostItems(ctx, invoiceLines, cfg.CostItemsTable)
		if err != nil {
			log.Errorf(err, "failed to insert cost item into bigquery")
			panic(err)
		}
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
