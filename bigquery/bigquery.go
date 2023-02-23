package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"

	"github.com/nais/aiven-cost/currency"
	"github.com/nais/aiven-cost/log"
)

const (
	ProjectID          = "nais-io"
	Dataset            = "aiven_cost"
	TableCurrencyRates = "currency_rates"
	Table              = "costitems"
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
	return &Client{
		client: client,
	}
}

func (c *Client) CreateOrUpdateCurrencyRates(ctx context.Context, currencyRates []currency.CurrencyRate) error {
	dataset := c.client.Dataset(Dataset)
	err := dataset.Table(TableCurrencyRates).Delete(ctx)
	if err != nil {
		return err
	}

	schema, err := bigquery.InferSchema(currency.CurrencyRate{})
	if err != nil {
		log.Errorf(err, "failed to infer schema")
		return err
	}

	if err := dataset.Table(TableCurrencyRates).Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		log.Errorf(err, "failed to create table")
		return err
	}

	if err != nil {
		log.Errorf(err, "failed to create table %s", TableCurrencyRates)
		return err
	}

	/*for currencyRate := range currencyRates {
		err = dataset.Table(TableCurrencyRates).Inserter().Put(ctx, currencyRate)
		if err != nil {
			log.Errorf(err, "failed to insert currency rate")
			return err
		}
	}*/
	return nil
}
