package bigquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	for i := 0; i < len(rates); i += insertBatchSize {
		batch := rates[i:min(i+insertBatchSize, len(rates))]
		if err := c.insertCurrencyRatesBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) insertCurrencyRatesBatch(ctx context.Context, rates []CurrencyRate) error {
	table := "`" + c.client.Project() + "." + c.dataset + "." + c.currencyTable + "`"
	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(table)
	sb.WriteString(" (date, usdeur, usdnok) VALUES ")
	for i, r := range rates {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "('%s','%s','%s')",
			bqEscape(r.Date),
			bqEscape(r.USDEUR),
			bqEscape(r.USDNOK),
		)
	}
	job, err := c.client.Query(sb.String()).Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to run insert currency rates query: %w", err)
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for insert currency rates job: %w", err)
	}
	if err := status.Err(); err != nil {
		return fmt.Errorf("insert currency rates job failed: %w", err)
	}
	return nil
}
