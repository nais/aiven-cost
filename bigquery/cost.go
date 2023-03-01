package bigquery

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/nais/aiven-cost/log"
	"google.golang.org/api/iterator"
)

func (c *Client) FetchCostItemIDAndStatus(ctx context.Context) (map[string]string, error) {
	log.Infof("query: %s", "SELECT DISTINCT invoice_id, status FROM "+c.cfg.ProjectID+"."+c.cfg.Dataset+"."+c.cfg.CostItemsTable)
	q := c.client.Query(`SELECT DISTINCT invoice_id, status FROM ` + c.cfg.ProjectID + "." + c.cfg.Dataset + "." + c.cfg.CostItemsTable)
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

func (c *Client) GetOldestDateFromCostItems(ctx context.Context) (time.Time, error) {
	t := time.Time{}
	q := c.client.Query("SELECT distinct date FROM " + c.cfg.ProjectID + "." + c.cfg.Dataset + "." + c.cfg.CostItemsTable + " ORDER BY date ASC LIMIT 1")
	it, err := q.Read(ctx)
	if err != nil {
		return t, err
	}

	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return t, err
		}
		date, err := time.Parse("2006-01", values[0].(string))
		if err != nil {
			return t, err
		}
		t = date
	}
	return t, nil
}

func (c *Client) DeleteUnpaid(ctx context.Context) error {
	q := c.client.Query("DELETE FROM " + c.cfg.ProjectID + "." + c.cfg.Dataset + "." + c.cfg.CostItemsTable + " WHERE status in ('estimate', 'mailed')")
	_, err := q.Read(ctx)
	if err != nil {
		log.Errorf(err, "failed to delete unpaid cost items")
		panic(err)
	}

	return nil
}

func (c *Client) InsertCostItems(ctx context.Context, lines []Line, tableName string) error {
	// log.Infof("Inserting cost item for: %s", line.InvoiceId)
	err := c.client.Dataset(c.cfg.Dataset).Table(c.cfg.CostItemsTable).Inserter().Put(ctx, lines)
	if err != nil {
		log.Errorf(err, "failed to insert cost item")
		return err
	}
	return nil
}
