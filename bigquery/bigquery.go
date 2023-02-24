package bigquery

import (
	"context"
	"errors"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"

	"github.com/nais/aiven-cost/currency"
	"github.com/nais/aiven-cost/log"
)

const (
	ProjectID = "nais-io"
	Dataset   = "aiven_cost"
)

type Client struct {
	client *bigquery.Client
}

func New(ctx context.Context) *Client {
	client, err := bigquery.NewClient(ctx, ProjectID)
	if err != nil {
		log.Errorf(err, "Failed to create client: %v")
		panic(err)
	}
	defer client.Close()
	return &Client{
		client: client,
	}
}

func (c *Client) TruncateCurrencyTable(ctx context.Context, tableName string) {
	q := c.client.Query(`DELETE FROM ` + ProjectID + "." + Dataset + "." + tableName + ` WHERE 1=1`)
	it, err := q.Read(ctx)
	if err != nil {
		log.Errorf(err, "failed to truncate table")
		panic(err)
	}
	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Errorf(err, "failed to truncate table")
			panic(err)
		}
	}
}

func (c *Client) CreateIfNotExists(ctx context.Context, schema any, tableName string) error {
	err := c.tableExists(ctx, ProjectID, Dataset, tableName)
	if err != nil {
		if err.Error() != "dataset or table not found" {
			log.Errorf(err, "failed to check if table exists")
			panic(err)
		} else {
			err := c.createTable(ctx, c.client.Dataset(Dataset), schema, tableName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) FetchCostItemIDandStatus(ctx context.Context, tableName string) (map[string]string, error) {
	q := c.client.Query(`SELECT DISTINCT invoice_id, status FROM ` + ProjectID + "." + Dataset + "." + tableName)
	it, err := q.Read(ctx)
	if err != nil {
		log.Errorf(err, "failed to fetch cost items")
		panic(err)
	}
	costItems := make(map[string]string)
	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Errorf(err, "failed to fetch cost items")
			panic(err)
		}
		costItems[values[0].(string)] = values[1].(string)
	}
	return costItems, nil
}

func (c *Client) CreateOrUpdateCurrencyRates(ctx context.Context, currencyRates []currency.CurrencyRate, tableName string) error {
	dataset := c.client.Dataset(Dataset)
	err := c.tableExists(ctx, ProjectID, Dataset, tableName)
	if err != nil {
		if err.Error() != "dataset or table not found" {
			log.Errorf(err, "failed to check if table exists")
			panic(err)
		} else if err.Error() == "dataset or table not found" {
			err := c.createTable(ctx, dataset, currency.CurrencyRate{}, tableName)
			if err != nil {
				return err
			}
		} else {
			c.TruncateCurrencyTable(ctx, tableName)
		}
	}
	c.client.Dataset(Dataset).Table(tableName)
	for _, currencyRate := range currencyRates {
		err = dataset.Table(tableName).Inserter().Put(ctx, currencyRate)
		if err != nil {
			log.Errorf(err, "failed to insert currency rate")
			return err
		}
	}
	return nil
}

func (c *Client) createTable(ctx context.Context, dataset *bigquery.Dataset, schema any, tableName string) error {
	s, err := bigquery.InferSchema(schema)
	if err != nil {
		log.Errorf(err, "failed to infer schema")
		return err
	}

	if err := dataset.Table(tableName).Create(ctx, &bigquery.TableMetadata{Schema: s}); err != nil {
		log.Errorf(err, "failed to create table")
		return err
	}
	return nil
}

// tableExists checks wheter a table exists on a given dataset.
func (c *Client) tableExists(ctx context.Context, projectID, datasetID, tableID string) error {
	tableRef := c.client.Dataset(datasetID).Table(tableID)
	if _, err := tableRef.Metadata(ctx); err != nil {
		if e, ok := err.(*googleapi.Error); ok {
			if e.Code == http.StatusNotFound {
				return errors.New("dataset or table not found")
			}
		}
	}
	return nil
}
