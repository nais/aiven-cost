package aiven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/sirupsen/logrus"
)

type Client struct {
	client         *http.Client
	apiHost        string
	apiToken       string
	billingGroupID string
	logger         *logrus.Logger
}

func New(apiHost, token, billingGroupID string, logger *logrus.Logger) *Client {
	return &Client{
		client:         http.DefaultClient,
		apiHost:        apiHost,
		apiToken:       token,
		billingGroupID: billingGroupID,
		logger:         logger,
	}
}

func (c *Client) do(ctx context.Context, v any, method, path string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+c.apiHost+path, body)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "aivenv1 "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if method == http.MethodGet && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) GetInvoices(ctx context.Context, billingGroupId string) ([]Invoice, error) {
	invoices := struct {
		Invoices []Invoice `json:"invoices"`
	}{}

	if err := c.do(ctx, &invoices, http.MethodGet, "/v1/billing-group/"+billingGroupId+"/invoice", nil); err != nil {
		return nil, err
	}

	return invoices.Invoices, nil
}

func parseServiceType(line InvoiceLine) string {
	switch line.LineType {
	case "support_charge", "extra_charge":
		return "support"
	case "credit_consumption":
		return "credit"
	}

	if strings.HasPrefix(line.ServiceName, "opensearch-") {
		return "opensearch"
	}

	return line.ServiceType
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
	}
	return team
}

func fetchServiceTags(serviceType string) bool {
	switch serviceType {
	case "support_charge", "extra_charge", "credit_consumption":
		return false
	}
	return true
}

func (c *Client) GetInvoiceLines(ctx context.Context, billingGroupId string, invoice Invoice) ([]bigquery.Line, error) {
	invoiceLines := struct {
		InvoiceLines []InvoiceLine `json:"lines"`
	}{}
	ret := []bigquery.Line{}

	if err := c.do(ctx, &invoiceLines, http.MethodGet, "/v1/billing-group/"+billingGroupId+"/invoice/"+invoice.InvoiceId+"/lines", nil); err != nil {
		return nil, err
	}

	for _, line := range invoiceLines.InvoiceLines {
		tags := Tags{}
		if fetchServiceTags(line.ServiceType) {
			t, err := c.GetServiceTags(ctx, line.ProjectName, line.ServiceName)
			if err != nil {
				c.logger.
					WithFields(logrus.Fields{
						"project": line.ProjectName,
						"service": line.ServiceName,
					}).
					WithError(err).
					Warnf("failed to get service tags")
			}
			tags = t
		}

		ret = append(ret, bigquery.Line{
			BillingGroupId: billingGroupId,
			InvoiceId:      invoice.InvoiceId,
			ProjectName:    line.ProjectName,
			Environment:    tags.Environment,
			Team:           parseTeam(tags.Team, line.ServiceType),
			Service:        parseServiceType(line),
			ServiceName:    line.ServiceName,
			Tenant:         parseTenant(tags.Tenant),
			Status:         invoice.Status,
			Cost:           line.Cost,
			Currency:       line.Currency,
			Date:           fmt.Sprintf("%02d-%02d", line.TimestampBegin.Year(), line.TimestampBegin.Month()),
			NumberOfDays:   daysIn(line.TimestampBegin.Month(), line.TimestampBegin.Year()),
		})

	}

	return ret, nil
}

func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (c *Client) GetServiceTags(ctx context.Context, projectName, serviceName string) (Tags, error) {
	tags := struct {
		Tags Tags `json:"tags"`
	}{}
	if err := c.do(ctx, &tags, http.MethodGet, "/v1/project/"+projectName+"/service/"+serviceName+"/tags", nil); err != nil {
		return Tags{}, err
	}

	return tags.Tags, nil
}

func (c *Client) GetInvoiceIDs(ctx context.Context) (map[string]string, error) {
	ret := make(map[string]string)

	invoices, err := c.GetInvoices(ctx, c.billingGroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoices for billing group %s", c.billingGroupID)
	}
	for _, invoice := range invoices {
		ret[invoice.InvoiceId] = c.billingGroupID
	}

	return ret, nil
}

func (c *Client) GetInvoice(ctx context.Context, billingGroupId, invoiceId string) (Invoice, error) {
	invoice := struct {
		Invoice Invoice `json:"invoice"`
	}{}

	if err := c.do(ctx, &invoice, http.MethodGet, "/v1/billing-group/"+billingGroupId+"/invoice/"+invoiceId, nil); err != nil {
		return Invoice{}, err
	}

	return invoice.Invoice, nil
}
