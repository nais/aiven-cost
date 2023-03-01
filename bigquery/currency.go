package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/nais/aiven-cost/log"
	"google.golang.org/api/iterator"
)

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

func (c *Client) InsertCurrencyRate(ctx context.Context, rate CurrencyRate) error {
	log.Infof("Inserting currency rate for: %s", rate.Date)

	existingCurrencyRates, err := c.GetCurrencyDates(ctx)
	if err != nil {
		log.Errorf(err, "failed to get existing currency rates")
		return err
	}

	if contains(existingCurrencyRates, rate.Date) {
		log.Infof("Currency rate for %s already exists, skipping", rate.Date)
		return nil
	}

	err = c.client.Dataset(c.cfg.Dataset).Table(c.cfg.CurrencyTable).Inserter().Put(ctx, rate)
	if err != nil {
		log.Errorf(err, "failed to insert currency rate")
		return err
	}
	return nil
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
