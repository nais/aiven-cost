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
	"github.com/nais/aiven-cost/currency"
	"github.com/nais/aiven-cost/log"
)

var cfg = struct {
	APIHost    string
	LogLevel   string
	AivenToken string
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
	flag.Parse()
	f, err := os.Create("aiven-cost.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	aivenClient := aiven.New(cfg.APIHost, cfg.AivenToken)

	currencyClient := currency.New()
	currency, err := currencyClient.GetRates(ctx)
	if err != nil {
		log.Errorf(err, "failed to get currency rates")
	}
	fmt.Println("Month\tEUR\tUSD")
	for _, v := range currency {
		fmt.Printf("%s\t%s\t%s\n", v.TimePeriod, v.EUR, v.USD)
	}

	bqClient := bigquery.New(ctx)
	err = bqClient.CreateOrUpdateCurrencyRates(ctx, currency)
	if err != nil {
		log.Errorf(err, "failed to create or update currency rates")
	}
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

				if invoiceDetail.LineType == "support_charge" && invoiceDetail.ServiceType == "" {
					invoiceDetail.ServiceType = "support"
					tags.Team = "nais"
				} else if invoiceDetail.LineType == "credit_consumption" {
					invoiceDetail.ServiceType = "credit"
					tags.Team = "nais"
				} else if invoiceDetail.LineType == "extra_charge" {
					if invoiceDetail.Description == "Priority support" {
						invoiceDetail.ServiceType = "priority_support"
						tags.Team = "nais"
					} else if strings.Contains(invoiceDetail.Description, "Missed DDS charges") {
						invoiceDetail.ServiceType = "missed_dds_charges"
						tags.Team = "nais"
					} else {
						// stupid to panic here, but we want to know about all the different extra charges
						panic(fmt.Sprintf("unknown extra charge %#v", invoiceDetail))
					}
				} else if invoiceDetail.LineType != "service_charge" {
					// stupid to panic here, but we want to know about all the different line types
					panic(fmt.Sprintf("unknown line type %#v", invoiceDetail.LineType))
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
				for _, line := range billingReport.SplitCostIme() {
					f.Write([]byte(line + "\n"))
					f.Sync()
				}
			}
		}
	}
}
