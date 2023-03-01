package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nais/aiven-cost/aiven"
	"github.com/nais/aiven-cost/bigquery"
	"github.com/nais/aiven-cost/config"
	"github.com/nais/aiven-cost/currency"
	"github.com/nais/aiven-cost/log"
)



func main() {
	ctx := context.Background()

	cfg := config.New()

	flag.StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")
	flag.StringVar(&cfg.ApiKey, "apikey", os.Getenv("APIKEY"), "Currency API token")
	flag.Parse()



	aivenClient := aiven.New(cfg.APIHost, cfg.AivenToken)

	bqClient := bigquery.New(ctx, cfg)

	currencyClient := currency.New(cfg.ApiKey)

	bqClient.CreateIfNotExists(ctx, bigquery.CurrencyRate{}, cfg.CurrencyTable)
	oldestDate, err := bqClient.GetOldestDateFromCostItems(ctx)
	if err != nil {
		log.Errorf(err, "failed to get currency dates")
	}
	rates := []bigquery.CurrencyRate{}
	last := false
	for {
		endDate := oldestDate.Add(time.Hour * 24 * 365)
		if oldestDate.Add(time.Hour * 24 * 365).After(time.Now()) {
			endDate = time.Now()
			last = true
		}

		resp, err := currencyClient.RatesPeriod(ctx, "USD", "EUR,NOK", oldestDate, endDate)
		if err != nil {
			log.Errorf(err, "failed to get currency rates")
		}

		for date, rate := range resp.Rates {
			rates = append(rates, bigquery.CurrencyRate{
				Date:   date,
				USDEUR: strconv.FormatFloat(rate.EUR, 'f', 6, 64),
				USDNOK: strconv.FormatFloat(rate.NOK, 'f', 6, 64),
			})
		}

		if last {
			break
		}
		oldestDate = oldestDate.Add(time.Hour * 24 * 365)
	}
	for _, rate := range rates {
		fmt.Printf("%#v\n", rate)
		err := bqClient.InsertCurrencyRate(ctx, rate)
		if err != nil {
			log.Errorf(err, "failed to insert currency rate")
			os.Exit(1)
		}
	}
	os.Exit(0)
	log.Infof("create bigquery table if not exists")
	err = bqClient.CreateIfNotExists(ctx, bigquery.Line{}, cfg.CostItemsTable)
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
