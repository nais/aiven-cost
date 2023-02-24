package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nais/aiven-cost/aiven"
	"github.com/nais/aiven-cost/bigquery"
	"github.com/nais/aiven-cost/billing"
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
	err = bqClient.CreateIfNotExists(ctx, bigquery.Line{}, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to create cost table")
	}
	bqInvoices, err := bqClient.FetchCostItemIDandStatus(ctx, cfg.CostItemsTable)
	if err != nil {
		log.Errorf(err, "failed to fetch cost item id and status")
		panic(err)
	}
	// fetch aiven invoice ids
	aivenInvoiceIDs, err := aivenClient.GetInvoiceIDs(ctx)
	if err != nil {
		log.Errorf(err, "failed to get invoice ids")
		panic(err)
	}

	filterPayedInvoices := func(aivenInvoiceIDs, bqInvoices map[string]string) map[string]string {
		unprocessedInvoices := make(map[string]string)
		for invoiceID, billingGroup := range aivenInvoiceIDs {
			if _, ok := bqInvoices[invoiceID]; ok && bqInvoices[invoiceID] == "paid" {
				continue
			}
			unprocessedInvoices[invoiceID] = billingGroup
		}
		return unprocessedInvoices
	}

	// filter out payed invoices
	unprocessedInvoices := filterPayedInvoices(aivenInvoiceIDs, bqInvoices)

	// fetch invoice details for each invoice and map to bigquery schema
	for invoiceID, billingGroup := range unprocessedInvoices {
		invoiceDetails, err := aivenClient.GetInvoiceDetails(ctx, invoiceID, billingGroup)
		if err != nil {
			log.Errorf(err, "failed to get invoice details for invoice %s", invoiceID)
			panic(err)
		}
		// map invoice details to bigquery schema
		bqInvoice := mapInvoiceToBigQuery(invoiceDetails)
		// insert invoice into bigquery
		err = bqClient.InsertCostItem(ctx, bqInvoice, cfg.CostItemsTable)
		if err != nil {
			log.Errorf(err, "failed to insert cost item into bigquery")
			panic(err)
		}
	}

	/*	err = bqClient.CreateOrUpdateCurrencyRates(ctx, currency, "currency_rates")
		if err != nil {
			log.Errorf(err, "failed to create or update currency rates")
		}
	*/
	billingGroups, err := aivenClient.GetBillingGroups(ctx)
	if err != nil {
		log.Errorf(err, "failed to get billing groups")
		panic(err)
	}

	for _, billingGroup := range billingGroups {
		fmt.Printf("%#v\n", billingGroup)

		invoices, err := aivenClient.GetInvoices(ctx, billingGroup.BillingGroupId)
		if err != nil {
			log.Errorf(err, "failed to get invoices for billing group %s", billingGroup.BillingGroupId)
			panic(err)
		}
		for _, invoice := range invoices {
			invoiceDetails, err := aivenClient.GetInvoiceDetails(ctx, billingGroup.BillingGroupId, invoice.InvoiceId)
			if err != nil {
				log.Errorf(err, "failed to get invoice details for billing group %s and invoice %s", billingGroup.BillingGroupId, invoice.InvoiceId)
				panic(err)
			}

			for _, invoiceDetail := range invoiceDetails {
				tags, err := aivenClient.GetServiceTags(ctx, invoiceDetail.ProjectName, invoiceDetail.ServiceName)
				if err != nil {
					log.Warnf("failed to get service tags for project %s and service %s: %v", invoiceDetail.ProjectName, invoiceDetail.ServiceName, err)
				}
				cost, err := strconv.ParseFloat(invoiceDetail.Cost, 64)
				if err != nil {
					log.Errorf(err, "failed to parse cost %s", invoiceDetail.Cost)
				}

				if invoiceDetail.ServiceType == "kafka" {
					tags.Team = "nais"
				}

				switch invoiceDetail.LineType {
				case "support_charge":
				case "extra_charge":
					invoiceDetail.ServiceType = "support"
					tags.Team = "nais"
				case "credit_consumption":
					invoiceDetail.ServiceType = "credit"
					tags.Team = "nais"
				default:
					invoiceDetail.ServiceType = invoiceDetail.LineType
				}

				billingReport := billing.AivenCostItem{
					BillingGroupId: string(billingGroup.BillingGroupId),
					InvoiceId:      invoice.InvoiceId,
					StartDate:      invoiceDetail.TimestampBegin,
					EndDate:        invoiceDetail.TimestampEnd,
					Service:        invoiceDetail.ServiceType,
					Cost:           cost,
					Tenant:         tags.Tenant,
					Team:           tags.Team,
					Environment:    tags.Environment,
					Currency:       strings.ToUpper(invoiceDetail.Currency),
				}
				for _, line := range billingReport.SplitCostPerDay() {
					f.Write([]byte(line + "\n"))
					f.Sync()
				}
			}
		}
	}
}
