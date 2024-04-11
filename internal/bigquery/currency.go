package bigquery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

func (c *Client) GetNewestDate(ctx context.Context) (time.Time, error) {
	var date time.Time
	q := c.client.Query("SELECT date FROM " + c.client.Project() + "." + c.dataset + "." + c.currencyTable + " ORDER BY date DESC LIMIT 1")
	it, err := q.Read(ctx)
	if err != nil {
		return date, err
	}

	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return date, err
		}

		date, err = time.Parse("2006-01-02", values[0].(string))
		if err != nil {
			return date, err
		}
	}
	return date, nil
}

func (c *Client) InsertCurrencyRates(ctx context.Context, rates []CurrencyRate) error {
	err := c.client.Dataset(c.dataset).Table(c.currencyTable).Inserter().Put(ctx, rates)
	if err != nil {
		return fmt.Errorf("failed to insert currency rate: %w", err)
	}
	return nil
}
