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
	orgID          string
	billingGroupID string
	logger         *logrus.Logger
}

func New(apiHost, token, orgID, billingGroupID string, logger *logrus.Logger) (*Client, error) {
	return &Client{
		client:         &http.Client{Timeout: 30 * time.Second},
		apiHost:        apiHost,
		apiToken:       token,
		orgID:          orgID,
		billingGroupID: billingGroupID,
		logger:         logger,
	}, nil
}

func (c *Client) GetInvoices(ctx context.Context) ([]Invoice, error) {
	path := fmt.Sprintf("/v1/organization/%s/invoices", c.orgID)
	body, err := c.doAivenGet(ctx, path)
	if err != nil {
		return nil, err
	}

	var out struct {
		Invoices []Invoice `json:"invoices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	return out.Invoices, nil
}

func ParseServiceType(lineType, serviceType string) string {
	switch lineType {
	case "support_charge", "extra_charge":
		return "support"
	case "credit_consumption":
		return "credit"

	default:
		return serviceType
	}
}

func ParseTenant(tenant string) string {
	if tenant == "" {
		return "nav"
	}

	return tenant
}

func ParseTeam(serviceID, serviceType string) string {
	switch serviceType {
	case "kafka", "support_charge", "extra_charge", "credit_consumption":
		return "nais"
	default:

		parts := strings.SplitN(serviceID, "-", 3)
		if len(parts) >= 2 {
			return parts[1]
		}

		return ""
	}
}

func AmountOfDaysInMonth(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (c *Client) doAivenGet(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("https://%s%s", c.apiHost, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) getOrgInvoiceLines(ctx context.Context, invoiceNumber string) ([]orgInvoiceLine, error) {
	path := fmt.Sprintf("/v1/organization/%s/invoices/%s/lines", c.orgID, invoiceNumber)
	body, err := c.doAivenGet(ctx, path)
	if err != nil {
		return nil, err
	}

	var out orgInvoiceLinesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	return out.Lines, nil
}

func (c *Client) GetInvoiceLines(ctx context.Context, invoice Invoice) ([]bigquery.Line, error) {
	ret := []bigquery.Line{}

	lines, err := c.getOrgInvoiceLines(ctx, invoice.InvoiceId)
	if err != nil {
		return nil, err
	}

	for _, line := range lines {
		timestampBegin, err := time.Parse(time.RFC3339, line.BeginTime)
		if err != nil {
			return nil, err
		}

		ret = append(ret, bigquery.Line{
			BillingGroupId: c.billingGroupID,
			InvoiceId:      invoice.InvoiceId,
			ProjectName:    line.ProjectID,
			Environment:    line.Tags.Environment,
			Team:           ParseTeam(line.ServiceID, line.ServiceType),
			Service:        ParseServiceType(line.LineType, line.ServiceType),
			ServiceName:    line.ServiceID,
			Tenant:         ParseTenant(line.Tags.Tenant),
			Status:         invoice.Status,
			Cost:           line.Total,
			Currency:       line.Currency,
			Date:           fmt.Sprintf("%02d-%02d", timestampBegin.Year(), timestampBegin.Month()),
			NumberOfDays:   AmountOfDaysInMonth(timestampBegin.Month(), timestampBegin.Year()),
		})
	}

	return ret, nil
}
