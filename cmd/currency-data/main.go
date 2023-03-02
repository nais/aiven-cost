package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nais/aiven-cost/bigquery"
	"github.com/nais/aiven-cost/config"
	"github.com/nais/aiven-cost/currency"
	"github.com/nais/aiven-cost/log"
)

func main() {
	ctx := context.Background()

	cfg := config.New()

	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.CurrencyToken, "currency-token", os.Getenv("CURRENCY_TOKEN"), "Currency API token")
	flag.Parse()

	bqClient := bigquery.New(ctx, cfg)
	currencyClient := currency.New(cfg.CurrencyToken)

	// TODO: Move to separate job, or move down in main
	err := bqClient.CreateIfNotExists(ctx, bigquery.CurrencyRate{}, cfg.CurrencyTable)
	if err != nil {
		panic(fmt.Errorf("failed creating cost table: %w", err))
	}

	fetchFrom := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	newestDate, err := bqClient.GetNewestDate(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to get newest date: %w", err))
	}
	if !newestDate.IsZero() {
		fetchFrom = newestDate.Add(time.Hour * 24)
	}

	rates := []bigquery.CurrencyRate{}
	last := false
	for {
		endDate := fetchFrom.Add(time.Hour * 24 * 365)
		if fetchFrom.Add(time.Hour * 24 * 365).After(time.Now()) {
			endDate = time.Now()
			last = true
		}

		resp, err := currencyClient.RatesPeriod(ctx, "USD", "EUR,NOK", fetchFrom, endDate)
		if err != nil {
			log.Errorf(err, "failed to get currency rates")
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
		fetchFrom = fetchFrom.Add(time.Hour * 24 * 365)
	}
	err = bqClient.InsertCurrencyRates(ctx, rates)
	if err != nil {
		log.Errorf(err, "failed to insert currency rate")
		os.Exit(1)
	}
}
