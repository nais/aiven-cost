package bigquery

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
)

const (
	gcpLocation = "europe-north1"
)

type Client struct {
	client         *bigquery.Client
	dataset        string
	costItemsTable string
	currencyTable  string
}

func New(ctx context.Context, projectID, dataset, costItemsTable, currencyTable string) (*Client, error) {
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client.Location = gcpLocation
	return &Client{
		client:         client,
		dataset:        dataset,
		costItemsTable: costItemsTable,
		currencyTable:  currencyTable,
	}, nil
}

func (c *Client) CreateTableIfNotExists(ctx context.Context, schema any, tableName string) error {
	if err := c.tableExists(ctx, tableName); err != nil {
		if err.Error() != "dataset or table not found" {
			return fmt.Errorf("failed to check if table exists: %w", err)
		}
	}

	if err := c.createTable(ctx, schema, tableName); err != nil {
		return err
	}

	return nil
}

func (c *Client) createTable(ctx context.Context, schema any, tableName string) error {
	s, err := bigquery.InferSchema(schema)
	if err != nil {
		return fmt.Errorf("failed to infer schema: %w", err)
	}

	if err := c.client.Dataset(c.dataset).Table(tableName).Create(ctx, &bigquery.TableMetadata{Schema: s}); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// tableExists checks wheter a table exists on a given dataset.
func (c *Client) tableExists(ctx context.Context, tableName string) error {
	tableRef := c.client.Dataset(c.dataset).Table(tableName)
	if _, err := tableRef.Metadata(ctx); err != nil {
		if e, ok := err.(*googleapi.Error); ok {
			if e.Code == http.StatusNotFound {
				return fmt.Errorf("dataset or table not found")
			}
		}
	}

	return nil
}
