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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) GetProjects(ctx context.Context) (Projects, error) {
	projects := struct {
		Projects Projects `json:"projects"`
	}{}

	if err := c.do(ctx, &projects, "GET", "/v1/project", nil); err != nil {
		return nil, err
	}
	return projects.Projects, nil
}

func (c *Client) GetServices(ctx context.Context, projectName string) (Services, error) {
	services := struct {
		Services Services `json:"services"`
	}{}

	if err := c.do(ctx, &services, "GET", "/v1/project/"+projectName+"/service", nil); err != nil {
		return nil, err
	}

	return services.Services, nil
}

func (c *Client) GetInvoices(ctx context.Context, billingGroupId string) ([]Invoice, error) {
	invoices := struct {
		Invoices []Invoice `json:"invoices"`
	}{}

	if err := c.do(ctx, &invoices, "GET", "/v1/billing-group/"+billingGroupId+"/invoice", nil); err != nil {
		return nil, err
	}

	return invoices.Invoices, nil
}

func (c *Client) GetInvoiceDetails(ctx context.Context, billingGroupId, invoiceId string) ([]InvoiceDetail, error) {
	invoiceDetails := struct {
		InvoiceDetails []InvoiceDetail `json:"lines"`
	}{}

	if err := c.do(ctx, &invoiceDetails, "GET", "/v1/billing-group/"+billingGroupId+"/invoice/"+invoiceId+"/lines", nil); err != nil {
		return nil, err
	}

	return invoiceDetails.InvoiceDetails, nil
}

func (c *Client) GetServiceTags(ctx context.Context, projectName, serviceName string) (Tags, error) {
	tags := struct {
		Tags Tags `json:"tags"`
	}{}
	if err := c.do(ctx, &tags, "GET", "/v1/project/"+projectName+"/service/"+serviceName+"/tags", nil); err != nil {
		return Tags{}, err
	}

	return tags.Tags, nil
}
