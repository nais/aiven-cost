package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nais/aiven-cost/internal/aiven"
	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/nais/aiven-cost/internal/config"
	"github.com/nais/aiven-cost/internal/log"
	"github.com/sirupsen/logrus"
)

const (
	exitCodeOK = iota
	exitCodeConfigError
	exitCodeLoggerError
	exitCodeRunError
)

func main() {
	cfg, err := config.New()
	if err != nil {
		fmt.Println("failed to create config")
		os.Exit(exitCodeConfigError)
	}

	logger, err := log.New(cfg.Log.Format, cfg.Log.Level)
	if err != nil {
		fmt.Println("unable to create logger")
		os.Exit(exitCodeLoggerError)
	}

	err = run(cfg, logger)
	if err != nil {
		logger.WithError(err).Errorf("error in run()")
		os.Exit(exitCodeRunError)
	}

	os.Exit(exitCodeOK)
}

func run(cfg *config.Config, logger *logrus.Logger) error {
	ctx := context.Background()

	aivenClient := aiven.New(cfg.Aiven.ApiHost, cfg.Aiven.Token, cfg.Aiven.BillingGroupID, logger)
	bqClient, err := bigquery.New(ctx, cfg.BigQuery.ProjectID, cfg.BigQuery.Dataset, cfg.BigQuery.CostItemsTable, cfg.BigQuery.CurrencyTable)
	if err != nil {
		return fmt.Errorf("failed to create bigquery client: %w", err)
	}

	logger.Infof("create bigquery table if not exists")
	err = bqClient.CreateTableIfNotExists(ctx, bigquery.Line{}, cfg.BigQuery.CostItemsTable)
	if err != nil {
		return fmt.Errorf("failed to create cost table: %w", err)
	}

	logger.Infof("delete unpaid cost lines from bigquery")
	err = bqClient.DeleteUnpaid(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete unpaid cost lines: %w", err)
	}

	logger.Infof("fetch cost item id and status from bigquery")
	bqInvoices, err := bqClient.FetchCostItemIDAndStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch cost item id and status: %w", err)
	}

	logger.Infof("fetch aiven invoice ids")
	aivenInvoiceIDs, err := aivenClient.GetInvoiceIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get invoice ids: %w", err)
	}

	logger.Infof("filter out paid invoices")
	unprocessedInvoices := filterPaidInvoices(aivenInvoiceIDs, bqInvoices)
	logger.Infof("fetch invoice details from aiven and insert into bigquery for %d invoices of a total of %d", len(unprocessedInvoices), len(aivenInvoiceIDs))
	for invoiceID, billingGroup := range unprocessedInvoices {
		invoice, err := aivenClient.GetInvoice(ctx, billingGroup, invoiceID)
		if err != nil {
			return fmt.Errorf("failed to get invoice for invoice %s: %w", invoiceID, err)
		}

		invoiceLines, err := aivenClient.GetInvoiceLines(ctx, billingGroup, invoice)
		if err != nil {
			return fmt.Errorf("failed to get invoice details for invoice %s: %w", invoiceID, err)
		}

		err = bqClient.InsertCostItems(ctx, invoiceLines)
		if err != nil {
			return fmt.Errorf("failed to insert cost item into bigquery: %w", err)
		}
	}

	return nil
}

func filterPaidInvoices(aivenInvoiceIDs, bqInvoices map[string]string) map[string]string {
	unprocessedInvoices := make(map[string]string)
	for invoiceID, billingGroup := range aivenInvoiceIDs {
		if _, ok := bqInvoices[invoiceID]; ok && bqInvoices[invoiceID] == "paid" {
			continue
		}
		unprocessedInvoices[invoiceID] = billingGroup
	}
	return unprocessedInvoices
}
