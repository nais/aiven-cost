package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nais/aiven-cost/aiven"
	"golang.org/x/exp/slog"
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

type AivenCostItem struct {
	InvoiceId   string
	Environment string
	Team        string
	Date        time.Time
	Service     string
	Cost        float64
	Tenant      string
}

func main() {
	ctx := context.Background()

	flag.StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")
	flag.Parse()

	aivenClient := aiven.New(cfg.APIHost, cfg.AivenToken)
	projects, err := aivenClient.GetProjects(ctx)
	if err != nil {
		Errorf(err, "failed to get projects")
	}

	for _, project := range projects {
		fmt.Println(project.Name)
		invoices, err := aivenClient.GetInvoices(ctx, project.BillingGroup)
		if err != nil {
			Errorf(err, "failed to get billing")
			panic("Unable to continue without billing")
		}

		for _, invoice := range invoices {
			invoiceDetails, err := aivenClient.GetInvoiceDetails(ctx, project.BillingGroup, invoice.InvoiceId)
			if err != nil {
				Errorf(err, "failed to get billing")
				panic("Unable to continue without billing")
			}
			for _, invoiceDetail := range invoiceDetails {
				if invoiceDetail.ProjectName != project.Name {
					continue
				}
				tags, err := aivenClient.GetServiceTags(ctx, project.Name, invoiceDetail.ServiceName)
				if err != nil {
					Errorf(err, "failed to get service tags")
				}

				cost, err := strconv.ParseFloat(invoiceDetail.Cost, 64)
				if err != nil {
					Errorf(err, "failed to parse cost")
				}
				billingReport := AivenCostItem{
					InvoiceId:   invoice.InvoiceId,
					Date:        invoiceDetail.TimestampBegin,
					Service:     invoiceDetail.ServiceType,
					Cost:        cost,
					Tenant:      tags.Tenant,
					Team:        tags.Team,
					Environment: tags.Environment,
				}
				fmt.Printf("%#v\n", billingReport)

			}
		}

		/*services, err := aivenClient.GetServices(ctx, project.Name)
		if err != nil {
			Errorf(err, "failed to get services")
			panic("Unable to continue without services")
		}

		for _, s := range services {
			billingReport := AivenCostItem{}

			if s.Tags.Tenant != "" {
				billingReport.Tenant = s.Tags.Tenant
			}
			if s.Tags.Environment != "" {
				billingReport.Environment = s.Tags.Environment
			}
			if s.Tags.Team != "" {
				billingReport.Team = s.Tags.Team
			} else {
				billingReport.Team = "nais"
			}

			if billingReport.Tenant == "" {
				Infof("Tenant for service %q in %q is empty", s.ServiceName, project.Name)
			}
			if billingReport.Environment == "" {
				Infof("Environment for service %q in %q is empty", s.ServiceName, project.Name)
			}
			if billingReport.Team == "" {
				Errorf(fmt.Errorf("team is empty"), "Team for %q is empty", project.Name)
				Infof("Team for service %q in %q is empty", s.ServiceName, project.Name)
			}
			billingReport.CostInUSD =
			// fmt.Printf("%#v\n", billingReport)
		}*/

	}
}

func Infof(format string, args ...any) {
	slog.Default().Info(fmt.Sprintf(format, args...))
}

func Errorf(err error, format string, args ...any) {
	slog.Default().Error(fmt.Sprintf(format, args...), err)
}
