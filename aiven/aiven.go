package aiven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (c *Client) GetInvoiceDetails(ctx context.Context, billingGroupId, invoiceId string) ([]InvoiceDetail, error) {
	invoiceDetails := struct {
		InvoiceDetails []InvoiceDetail `json:"lines"`
	}{}

	if err := c.do(ctx, &invoiceDetails, http.MethodGet, "/v1/billing-group/"+billingGroupId+"/invoice/"+invoiceId+"/lines", nil); err != nil {
		return nil, err
	}

	return invoiceDetails.InvoiceDetails, nil
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
