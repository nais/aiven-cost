package bigquery

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/nais/aiven-cost/log"
	"google.golang.org/api/iterator"
)

func (c *Client) GetNewestDate(ctx context.Context) (time.Time, error) {
	var date time.Time
	q := c.client.Query("SELECT date FROM " + c.cfg.ProjectID + "." + c.cfg.Dataset + "." + c.cfg.CurrencyTable + " ORDER BY date DESC LIMIT 1")
	it, err := q.Read(ctx)
	if err != nil {
		return date, err
	}

	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
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

func (c *Client) GetCurrencyDates(ctx context.Context) ([]string, error) {
	var dates []string
	q := c.client.Query("SELECT date FROM " + c.cfg.ProjectID + "." + c.cfg.Dataset + "." + c.cfg.CurrencyTable + " ORDER BY date ASC")
	it, err := q.Read(ctx)
	if err != nil {
		return dates, err
	}

	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return dates, err
		}
		date := values[0].(string)
		if err != nil {
			return dates, err
		}
		dates = append(dates, date)
	}
	return dates, nil
}

func (c *Client) InsertCurrencyRates(ctx context.Context, rates []CurrencyRate) error {
	err := c.client.Dataset(c.cfg.Dataset).Table(c.cfg.CurrencyTable).Inserter().Put(ctx, rates)
	if err != nil {
		log.Errorf(err, "failed to insert currency rate")
		return err
	}
	return nil
}
