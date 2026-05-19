package bigquery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

const insertBatchSize = 500

func (c *Client) FetchDistinctDates(ctx context.Context) (map[string]struct{}, error) {
	q := c.client.Query(`SELECT DISTINCT date FROM ` + c.client.Project() + "." + c.dataset + "." + c.costItemsTable)
	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch distinct dates: %w", err)
	}
	dates := make(map[string]struct{})
	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read distinct dates: %w", err)
		}
		if len(values) > 0 {
			dates[values[0].(string)] = struct{}{}
		}
	}
	return dates, nil
}

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

func (c *Client) FetchServiceTeams(ctx context.Context) (map[string]string, error) {
	q := c.client.Query(`SELECT DISTINCT project_name, service_name, team FROM ` + c.client.Project() + "." + c.dataset + "." + c.costItemsTable + ` WHERE team IS NOT NULL AND team != ''`)
	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service teams: %w", err)
	}
	teams := make(map[string]string)
	for {
		var values []bigquery.Value
		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read service teams: %w", err)
		}
		if len(values) == 3 && values[0] != nil && values[1] != nil && values[2] != nil {
			key := values[0].(string) + "/" + values[1].(string)
			teams[key] = values[2].(string)
		}
	}
	return teams, nil
}

func (c *Client) FetchLatestPaidDate(ctx context.Context) (string, error) {
	q := c.client.Query(`SELECT MAX(date) FROM ` + c.client.Project() + "." + c.dataset + "." + c.costItemsTable + ` WHERE status = 'paid'`)
	it, err := q.Read(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest paid date: %w", err)
	}
	var values []bigquery.Value
	if err := it.Next(&values); err != nil {
		return "", fmt.Errorf("failed to read latest paid date: %w", err)
	}
	if len(values) == 0 || values[0] == nil {
		return "", fmt.Errorf("no paid invoices found in bigquery")
	}
	return values[0].(string), nil
}

func (c *Client) DeleteUnpaid(ctx context.Context) error {
	q := c.client.Query("DELETE FROM " + c.client.Project() + "." + c.dataset + "." + c.costItemsTable + " WHERE status in ('estimate', 'mailed')")
	if _, err := q.Read(ctx); err != nil {
		return fmt.Errorf("failed to delete unpaid cost items: %w", err)
	}
	return nil
}

func (c *Client) InsertCostItems(ctx context.Context, lines []Line) error {
	for i := 0; i < len(lines); i += insertBatchSize {
		batch := lines[i:min(i+insertBatchSize, len(lines))]
		if err := c.insertCostItemsBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) insertCostItemsBatch(ctx context.Context, lines []Line) error {
	table := "`" + c.client.Project() + "." + c.dataset + "." + c.costItemsTable + "`"
	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(table)
	sb.WriteString(" (billing_group_id, invoice_id, project_name, environment, team, service, service_name, tenant, status, cost, currency, date, number_of_days) VALUES ")
	for i, l := range lines {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "('%s','%s','%s','%s','%s','%s','%s','%s','%s','%s','%s','%s',%d)",
			bqEscape(l.BillingGroupId),
			bqEscape(l.InvoiceId),
			bqEscape(l.ProjectName),
			bqEscape(l.Environment),
			bqEscape(l.Team),
			bqEscape(l.Service),
			bqEscape(l.ServiceName),
			bqEscape(l.Tenant),
			bqEscape(l.Status),
			bqEscape(l.Cost),
			bqEscape(l.Currency),
			bqEscape(l.Date),
			l.NumberOfDays,
		)
	}
	job, err := c.client.Query(sb.String()).Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to run insert cost items query: %w", err)
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for insert cost items job: %w", err)
	}
	if err := status.Err(); err != nil {
		return fmt.Errorf("insert cost items job failed: %w", err)
	}
	return nil
}

func bqEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

