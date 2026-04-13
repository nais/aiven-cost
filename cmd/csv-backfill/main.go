package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nais/aiven-cost/internal/aiven"
	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/nais/aiven-cost/internal/config"
	"github.com/nais/aiven-cost/internal/log"
	"github.com/sirupsen/logrus"
)

const (
	exitCodeOK = iota
	exitCodeConfigError
	exitCodeLoggerError
	exitCodeRunError
	exitCodeUsageError
)

// CSV column indices (0-based)
const (
	colProjectName  = 0
	colServiceName  = 1
	colServiceType  = 2
	colBeginTime    = 6
	colTotal        = 8
	colLineCurrency = 10
	colTags         = 14
)

// beginTimeLayouts lists the timestamp formats seen in Aiven CSV exports.
var beginTimeLayouts = []string{
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05Z",
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would be inserted without writing to BigQuery")
	fromStr := flag.String("from", "", "start of missing-month range, format YYYY-MM (default: January of current year)")
	toStr := flag.String("to", "", "end of missing-month range, format YYYY-MM (default: current month)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: csv-backfill [--dry-run] [--from YYYY-MM] [--to YYYY-MM] <file.csv> [...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	csvFiles := flag.Args()
	if len(csvFiles) == 0 {
		flag.Usage()
		os.Exit(exitCodeUsageError)
	}

	now := time.Now().UTC()

	from, err := parseYearMonth(*fromStr, time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --from value: %v\n", err)
		os.Exit(exitCodeUsageError)
	}

	to, err := parseYearMonth(*toStr, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --to value: %v\n", err)
		os.Exit(exitCodeUsageError)
	}

	cfg, err := config.New()
	if err != nil {
		fmt.Println("failed to create config")
		os.Exit(exitCodeConfigError)
	}

	logger, err := log.New(cfg.Log.Format, cfg.Log.Level)
	if err != nil {
		fmt.Println("unable to create logger")
		os.Exit(exitCodeLoggerError)
	}

	if err := run(cfg, logger, csvFiles, from, to, *dryRun); err != nil {
		logger.WithError(err).Errorf("error in run()")
		os.Exit(exitCodeRunError)
	}

	os.Exit(exitCodeOK)
}

func run(cfg *config.Config, logger *logrus.Logger, csvFiles []string, from, to time.Time, dryRun bool) error {
	ctx := context.Background()

	bqClient, err := bigquery.New(ctx, cfg.BigQuery.ProjectID, cfg.BigQuery.Dataset, cfg.BigQuery.CostItemsTable, cfg.BigQuery.CurrencyTable)
	if err != nil {
		return fmt.Errorf("failed to create bigquery client: %w", err)
	}

	// Find months that are already in BigQuery
	logger.Info("fetching distinct dates already in bigquery")
	existingDates, err := bqClient.FetchDistinctDates(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch existing dates from bigquery: %w", err)
	}

	// Build the full set of months we expect in [from..to]
	expectedMonths := monthRange(from, to)

	// Determine which months are missing
	missingMonths := make([]string, 0)
	for _, m := range expectedMonths {
		if _, ok := existingDates[m]; !ok {
			missingMonths = append(missingMonths, m)
		}
	}

	logger.Infof("date range: %s to %s (%d months)", from.Format("2006-01"), to.Format("2006-01"), len(expectedMonths))
	logger.Infof("months already in bigquery: %d", len(expectedMonths)-len(missingMonths))

	if len(missingMonths) == 0 {
		logger.Info("no missing months found — nothing to do")
		return nil
	}

	logger.Infof("missing months: %s", strings.Join(missingMonths, ", "))

	// Build a set for quick lookup
	missingSet := make(map[string]struct{}, len(missingMonths))
	for _, m := range missingMonths {
		missingSet[m] = struct{}{}
	}

	// Parse all CSV files and collect lines for missing months
	allLines := make([]bigquery.Line, 0)
	coveredMonths := make(map[string]int) // month -> row count

	for _, csvFile := range csvFiles {
		invoiceID := invoiceIDFromFilename(csvFile)
		logger.Infof("processing %s (invoice_id: %s)", csvFile, invoiceID)

		lines, err := parseCSV(csvFile, invoiceID, cfg.Aiven.BillingGroupID, missingSet, logger)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", csvFile, err)
		}

		for _, l := range lines {
			coveredMonths[l.Date]++
		}
		allLines = append(allLines, lines...)
		logger.Infof("  → %d rows to insert from %s", len(lines), csvFile)
	}

	if len(allLines) == 0 {
		logger.Warn("CSV files provided no rows for the missing months")
		return nil
	}

	// Report coverage
	stillMissing := make([]string, 0)
	for _, m := range missingMonths {
		if _, covered := coveredMonths[m]; !covered {
			stillMissing = append(stillMissing, m)
		}
	}

	logger.Infof("total rows to insert: %d", len(allLines))
	for m, count := range coveredMonths {
		logger.Infof("  %s: %d rows", m, count)
	}
	if len(stillMissing) > 0 {
		logger.Warnf("months still not covered by any CSV: %s", strings.Join(stillMissing, ", "))
	}

	if dryRun {
		logger.Info("--dry-run enabled: skipping BigQuery insert")
		for _, l := range allLines {
			logger.WithFields(logrus.Fields{
				"invoice_id":   l.InvoiceId,
				"project_name": l.ProjectName,
				"service_name": l.ServiceName,
				"service":      l.Service,
				"team":         l.Team,
				"tenant":       l.Tenant,
				"environment":  l.Environment,
				"date":         l.Date,
				"cost":         l.Cost,
				"currency":     l.Currency,
				"status":       l.Status,
			}).Info("[dry-run] would insert row")
		}
		return nil
	}

	logger.Infof("inserting %d rows into bigquery", len(allLines))
	if err := bqClient.InsertCostItems(ctx, allLines); err != nil {
		return fmt.Errorf("failed to insert rows into bigquery: %w", err)
	}
	logger.Info("insert complete")

	return nil
}

// parseCSV reads a CSV file and returns BigQuery lines for rows whose month is in missingSet.
// Rows with an empty service_type are skipped (discount/commitment lines).
func parseCSV(path, invoiceID, billingGroupID string, missingSet map[string]struct{}, logger *logrus.Logger) ([]bigquery.Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	// Read and discard header row
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	lines := make([]bigquery.Line, 0)
	rowNum := 1

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNum, err)
		}
		rowNum++

		serviceType := record[colServiceType]
		// Skip discount/commitment lines (empty service_type)
		if serviceType == "" {
			continue
		}

		beginTime, err := parseBeginTime(record[colBeginTime])
		if err != nil {
			logger.WithError(err).Warnf("row %d: skipping — cannot parse begin_time %q", rowNum, record[colBeginTime])
			continue
		}

		date := fmt.Sprintf("%04d-%02d", beginTime.Year(), beginTime.Month())
		if _, ok := missingSet[date]; !ok {
			// This row belongs to a month already in BQ — skip it
			continue
		}

		tags := parseTags(record[colTags])
		team := aiven.ParseTeam(tags["team"], serviceType)
		service := aiven.ParseServiceType("", serviceType)
		tenant := aiven.ParseTenant(tags["tenant"])
		environment := tags["environment"]

		lines = append(lines, bigquery.Line{
			BillingGroupId: billingGroupID,
			InvoiceId:      invoiceID,
			ProjectName:    record[colProjectName],
			ServiceName:    record[colServiceName],
			Service:        service,
			Team:           team,
			Tenant:         tenant,
			Environment:    environment,
			Status:         "paid",
			Cost:           record[colTotal],
			Currency:       record[colLineCurrency],
			Date:           date,
			NumberOfDays:   aiven.AmountOfDaysInMonth(beginTime.Month(), beginTime.Year()),
		})
	}

	return lines, nil
}

// parseTags parses the Aiven CSV tags field.
// Format: "billing:environment=dev;billing:tenant=atil;billing:team=myteam"
// Returns a map with keys: "environment", "tenant", "team".
func parseTags(raw string) map[string]string {
	result := make(map[string]string)
	if raw == "" {
		return result
	}
	for part := range strings.SplitSeq(raw, ";") {
		// strip "billing:" prefix
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimPrefix(strings.TrimSpace(key), "billing:")
		result[key] = strings.TrimSpace(val)
	}
	return result
}

// parseBeginTime attempts multiple layouts to parse the begin_time field.
func parseBeginTime(s string) (time.Time, error) {
	for _, layout := range beginTimeLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised begin_time format: %q", s)
}

// invoiceIDFromFilename returns the base filename without extension.
// e.g. "path/to/invoice-da23c-67.csv" → "invoice-da23c-67"
func invoiceIDFromFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// monthRange returns all YYYY-MM strings from start to end (inclusive), stepping by month.
func monthRange(start, end time.Time) []string {
	months := make([]string, 0)
	cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	endFirst := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cur.After(endFirst) {
		months = append(months, fmt.Sprintf("%04d-%02d", cur.Year(), cur.Month()))
		cur = cur.AddDate(0, 1, 0)
	}
	return months
}

// parseYearMonth parses a "YYYY-MM" string. Returns defaultVal if input is empty.
func parseYearMonth(s string, defaultVal time.Time) (time.Time, error) {
	if s == "" {
		return defaultVal, nil
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
