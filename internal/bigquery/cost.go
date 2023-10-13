package bigquery

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

func (c *Client) FetchCostItemIDAndStatus(ctx context.Context) (map[string]string, error) {
	q := c.client.Query(`SELECT DISTINCT invoice_id, status FROM ` + c.client.Project() + "." + c.dataset + "." + c.costItemsTable)
	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cost items: %w", err)
	}
	costItems := make(map[string]string)
	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch cost items: %w", err)
		}
		costItems[values[0].(string)] = values[1].(string)
	}
	return costItems, nil
}

func (c *Client) DeleteUnpaid(ctx context.Context) error {
	q := c.client.Query("DELETE FROM " + c.client.Project() + "." + c.dataset + "." + c.costItemsTable + " WHERE status in ('estimate', 'mailed')")
	if _, err := q.Read(ctx); err != nil {
		return fmt.Errorf("failed to delete unpaid cost items: %w", err)
	}
	return nil
}

func (c *Client) InsertCostItems(ctx context.Context, lines []Line) error {
	err := c.client.Dataset(c.dataset).Table(c.costItemsTable).Inserter().Put(ctx, lines)
	if err != nil {
		return fmt.Errorf("failed to insert cost item: %w", err)
	}
	return nil
}
