package bigquery

import (
	"context"
	"errors"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"github.com/nais/aiven-cost/config"
	"github.com/nais/aiven-cost/log"
)

type Client struct {
	client *bigquery.Client
	cfg    *config.Config
}

func New(ctx context.Context, cfg *config.Config, location string) *Client {
	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		log.Errorf(err, "Failed to create client: %v")
		panic(err)
	}
	defer client.Close()
	client.Location = location
	return &Client{
		client: client,
		cfg:    cfg,
	}
}

func (c *Client) CreateIfNotExists(ctx context.Context, schema any, tableName string) error {
	err := c.tableExists(ctx, tableName)
	if err != nil {
		if err.Error() != "dataset or table not found" {
			log.Errorf(err, "failed to check if table exists")
			panic(err)
		} else {
			err := c.createTable(ctx, schema, tableName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) createTable(ctx context.Context, schema any, tableName string) error {
	s, err := bigquery.InferSchema(schema)
	if err != nil {
		log.Errorf(err, "failed to infer schema")
		return err
	}

	if err := c.client.Dataset(c.cfg.Dataset).Table(tableName).Create(ctx, &bigquery.TableMetadata{Schema: s}); err != nil {
		log.Errorf(err, "failed to create table")
		return err
	}
	return nil
}

// tableExists checks wheter a table exists on a given dataset.
func (c *Client) tableExists(ctx context.Context, tableName string) error {
	tableRef := c.client.Dataset(c.cfg.Dataset).Table(tableName)
	if _, err := tableRef.Metadata(ctx); err != nil {
		if e, ok := err.(*googleapi.Error); ok {
			if e.Code == http.StatusNotFound {
				return errors.New("dataset or table not found")
			}
		}
	}
	return nil
}
