package aiven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	orgID          string
	billingGroupID string
	logger         *logrus.Logger
}

func New(apiHost, token, orgID, billingGroupID string, logger *logrus.Logger) (*Client, error) {
	client, err := aivenclient.NewClient(aivenclient.TokenOpt(token), aivenclient.UserAgentOpt("nais-aiven-cost"))
	if err != nil {
		return nil, err
	}
	return &Client{
		client:         &http.Client{Timeout: 30 * time.Second},
		aivenClient:    client,
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

type orgInvoiceLinesResponse struct {
	Lines []orgInvoiceLine `json:"lines"`
}

type orgInvoiceLine struct {
	BeginTime   string            `json:"begin_time"`
	Cloud       string            `json:"cloud"`
	Currency    string            `json:"currency"`
	Description string            `json:"description"`
	EndTime     string            `json:"end_time"`
	LineType    string            `json:"line_type"`
	Plan        string            `json:"plan"`
	ProjectID   string            `json:"project_id"`
	ServiceID   string            `json:"service_id"`
	ServiceType string            `json:"service_type"`
	Tags        map[string]string `json:"tags"`
	Total       string            `json:"total"`
	TotalUSD    string            `json:"total_usd"`
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
		tags := Tags{}
		if line.ProjectID != "" && line.ServiceID != "" {
			fetchedTags, err := c.GetServiceTags(ctx, line.ProjectID, line.ServiceID)
			if err != nil {
				c.logger.
					WithFields(logrus.Fields{
						"project": line.ProjectID,
						"service": line.ServiceID,
					}).
					WithError(err).
					Warnf("failed to get service tags")
			}
			tags = fetchedTags
		}

		timestampBegin, err := time.Parse(time.RFC3339, line.BeginTime)
		if err != nil {
			return nil, err
		}

		ret = append(ret, bigquery.Line{
			BillingGroupId: c.billingGroupID,
			InvoiceId:      invoice.InvoiceId,
			ProjectName:    line.ProjectID,
			Environment:    tags.Environment,
			Team:           parseTeam(tags.Team, line.ServiceType),
			Service:        parseServiceType(line.LineType, line.ServiceType),
			ServiceName:    line.ServiceID,
			Tenant:         parseTenant(tags.Tenant),
			Status:         invoice.Status,
			Cost:           line.Total,
			Currency:       line.Currency,
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
