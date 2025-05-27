package aiven

import (
	"context"
	"fmt"
	"net/http"
	"time"

	aivenclient "github.com/aiven/go-client-codegen"

	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/sirupsen/logrus"
)

type Client struct {
	client         *http.Client
	aivenClient    aivenclient.Client
	apiHost        string
	apiToken       string
	billingGroupID string
	logger         *logrus.Logger
}

func New(apiHost, token, billingGroupID string, logger *logrus.Logger) (*Client, error) {
	client, err := aivenclient.NewClient(aivenclient.TokenOpt(token), aivenclient.UserAgentOpt("nais-aiven-cost"))
	if err != nil {
		return nil, err
	}
	return &Client{
		client:         http.DefaultClient,
		aivenClient:    client,
		apiHost:        apiHost,
		apiToken:       token,
		billingGroupID: billingGroupID,
		logger:         logger,
	}, nil
}

func (c *Client) GetInvoices(ctx context.Context) ([]Invoice, error) {
	invoices := struct {
		Invoices []Invoice `json:"invoices"`
	}{}
	aivenInvoices, err := c.aivenClient.BillingGroupInvoiceList(ctx, c.billingGroupID)
	if err != nil {
		return nil, err
	}
	for _, aivenInvoice := range aivenInvoices {
		invoices.Invoices = append(invoices.Invoices, Invoice{
			InvoiceId:   aivenInvoice.InvoiceNumber,
			TotalIncVat: aivenInvoice.TotalIncVat,
			Status:      string(aivenInvoice.State),
		})
	}
	return invoices.Invoices, nil
}

func parseServiceType(lineType, serviceType string) string {
	switch lineType {
	case "support_charge", "extra_charge":
		return "support"
	case "credit_consumption":
		return "credit"

	default:
		return serviceType
	}
}

func parseTenant(tenant string) string {
	if tenant == "" {
		return "nav"
	}
	return tenant
}

func parseTeam(team, serviceType string) string {
	switch serviceType {
	case "kafka", "support_charge", "extra_charge", "credit_consumption":
		return "nais"
	default:
		return team
	}
}

func (c *Client) GetInvoiceLines(ctx context.Context, invoice Invoice) ([]bigquery.Line, error) {
	ret := []bigquery.Line{}

	aivenInvoiceLines, err := c.aivenClient.BillingGroupInvoiceLinesList(ctx, c.billingGroupID, invoice.InvoiceId)
	if err != nil {
		return nil, err
	}

	for _, line := range aivenInvoiceLines {
		tags := Tags{}
		if line.ProjectName != nil && line.ServiceName != nil {
			fetchedTags, err := c.GetServiceTags(ctx, *line.ProjectName, *line.ServiceName)
			if err != nil {
				c.logger.
					WithFields(logrus.Fields{
						"project": line.ProjectName,
						"service": line.ServiceName,
					}).
					WithError(err).
					Warnf("failed to get service tags")
			}
			tags = fetchedTags
		}

		timestampBegin, err := time.Parse(time.RFC3339, *line.TimestampBegin)
		if err != nil {
			return nil, err
		}
		ret = append(ret, bigquery.Line{
			BillingGroupId: c.billingGroupID,
			InvoiceId:      invoice.InvoiceId,
			ProjectName:    *line.ProjectName,
			Environment:    tags.Environment,
			Team:           parseTeam(tags.Team, string(line.ServiceType)),
			Service:        parseServiceType(string(line.LineType), string(line.ServiceType)),
			ServiceName:    *line.ServiceName,
			Tenant:         parseTenant(tags.Tenant),
			Status:         invoice.Status,
			Cost:           *line.LineTotalLocal,
			Currency:       *line.LocalCurrency,
			Date:           fmt.Sprintf("%02d-%02d", timestampBegin.Year(), timestampBegin.Month()),
			NumberOfDays:   amountOfDaysInMonth(timestampBegin.Month(), timestampBegin.Year()),
		})

	}

	return ret, nil
}

func amountOfDaysInMonth(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (c *Client) GetServiceTags(ctx context.Context, projectName, serviceName string) (Tags, error) {
	tags := struct {
		Tags Tags `json:"tags"`
	}{}

	resp, err := c.aivenClient.ProjectServiceTagsList(ctx, projectName, serviceName)
	if err != nil {
		return Tags{}, err
	}

	if val, ok := resp["tenant"]; ok {
		tags.Tags.Tenant = val
	}
	if val, ok := resp["environment"]; ok {
		tags.Tags.Environment = val
	}
	if val, ok := resp["team"]; ok {
		tags.Tags.Team = val
	}

	return tags.Tags, nil
}
