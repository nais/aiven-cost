package aiven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nais/aiven-cost/bigquery"
	"github.com/nais/aiven-cost/log"
)

type Client struct {
	client  *http.Client
	APIHost string
	Token   string
}

func New(apiHost, token string) *Client {
	return &Client{
		client:  http.DefaultClient,
		APIHost: apiHost,
		Token:   token,
	}
}

func (c *Client) do(ctx context.Context, v any, method, path string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+c.APIHost+path, body)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "aivenv1 "+c.Token)

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

func (c *Client) GetBillingGroups(ctx context.Context) (BillingGroups, error) {
	billingGroups := struct {
		BillingGroups BillingGroups `json:"billing_groups"`
	}{}

	if err := c.do(ctx, &billingGroups, http.MethodGet, "/v1/billing-group", nil); err != nil {
		return nil, err
	}

	return billingGroups.BillingGroups, nil
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

func parseServiceType(serviceType string) string {
	switch serviceType {
	case "support_charge":
	case "extra_charge":
		return "support"
	case "credit_consumption":
		return "credit"
	}
	return serviceType
}

func (c *Client) GetInvoiceDetails(ctx context.Context, billingGroupId, invoiceId string) ([]bigquery.Line, error) {
	invoiceLines := struct {
		InvoiceLines []InvoiceLine `json:"lines"`
	}{}
	ret := []bigquery.Line{}

	if err := c.do(ctx, &invoiceLines, http.MethodGet, "/v1/billing-group/"+billingGroupId+"/invoice/"+invoiceId+"/lines", nil); err != nil {
		return nil, err
	}

	for _, invoiceDetail := range invoiceLines.InvoiceLines {
		tags, err := c.GetServiceTags(ctx, invoiceDetail.ProjectName, invoiceDetail.ServiceName)
		if err != nil {
			log.Warnf("failed to get service tags for project %s and service %s: %v", invoiceDetail.ProjectName, invoiceDetail.ServiceName, err)
		}
		ret = append(ret, bigquery.Line{
			InvoiceId:   invoiceId,
			Environment: tags.Environment,
			Team:        tags.Team,
			Service:     parseServiceType(invoiceDetail.ServiceType),
			Tenant:      tags.Tenant,
			Status:      invoiceDetail.Status,
			Cost:        invoiceDetail.Cost,
			Date:        invoiceDetail.Date,
		})

	}

	return ret, nil
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
	billingGroups, err := c.GetBillingGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get billing groups %v", err)
	}
	ret := make(map[string]string)

	for _, billingGroup := range billingGroups {
		invoices, err := c.GetInvoices(ctx, billingGroup.BillingGroupId)
		if err != nil {
			return nil, fmt.Errorf("failed to get invoices for billing group %s", billingGroup.BillingGroupId)
		}
		for _, invoice := range invoices {
			ret[invoice.InvoiceId] = billingGroup.BillingGroupId
		}
	}
	return ret, nil
}
