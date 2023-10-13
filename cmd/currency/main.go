package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nais/aiven-cost/internal/bigquery"
	"github.com/nais/aiven-cost/internal/config"
	"github.com/nais/aiven-cost/internal/currency"
	"github.com/nais/aiven-cost/internal/log"
)

const (
	exitCodeOK = iota
	exitCodeConfigError
	exitCodeLoggerError
	exitCodeRunError
)

func main() {
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

	err = run(cfg)
	if err != nil {
		logger.WithError(err).Error("error in run()")
		os.Exit(exitCodeRunError)
	}

	os.Exit(exitCodeOK)
}

func run(cfg *config.Config) error {
	ctx := context.Background()

	bqClient, err := bigquery.New(ctx, cfg.BigQuery.ProjectID, cfg.BigQuery.Dataset, cfg.BigQuery.CostItemsTable, cfg.BigQuery.CurrencyTable)
	if err != nil {
		return fmt.Errorf("failed to create bigquery client: %w", err)
	}

	currencyClient := currency.New(cfg.Currency.Token)

	// TODO: Move to separate job, or move down in main
	err = bqClient.CreateTableIfNotExists(ctx, bigquery.CurrencyRate{}, cfg.BigQuery.CurrencyTable)
	if err != nil {
		return fmt.Errorf("failed creating cost table: %w", err)
	}

	fetchFrom := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	newestDate, err := bqClient.GetNewestDate(ctx)
	if err != nil {
		return fmt.Errorf("failed to get newest date: %w", err)
	}

	if !newestDate.IsZero() {
		fetchFrom = newestDate.Add(time.Hour * 24)
	}

	rates := []bigquery.CurrencyRate{}
	last := false
	for {
		endDate := fetchFrom.Add(time.Hour * 24 * 365)
		if endDate.After(time.Now()) {
			endDate = time.Now()
			last = true
		}

		resp, err := currencyClient.RatesPeriod(ctx, "USD", "EUR,NOK", fetchFrom, endDate)
		if err != nil {
			return fmt.Errorf("failed to get currency rates: %w", err)
		}

		for date, rate := range resp.Rates {
			rates = append(rates, bigquery.CurrencyRate{
				Date:   date,
				USDEUR: strconv.FormatFloat(rate.EUR, 'f', 6, 64),
				USDNOK: strconv.FormatFloat(rate.NOK, 'f', 6, 64),
			})
		}

		if last {
			break
		}
		fetchFrom = endDate.Add(time.Hour * 24)
	}

	err = bqClient.InsertCurrencyRates(ctx, rates)
	if err != nil {
		return fmt.Errorf("failed to insert currency rate: %w", err)
	}

	return nil
}
