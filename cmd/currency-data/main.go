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

	flag.StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")
	flag.StringVar(&cfg.CurrencyToken, "currency-token", os.Getenv("CURRENCY_TOKEN"), "Currency API token")
	flag.Parse()

	bqClient := bigquery.New(ctx, cfg)
	currencyClient := currency.New(cfg.CurrencyToken)

	// TODO: Move to separate job, or move down in main
	bqClient.CreateIfNotExists(ctx, bigquery.CurrencyRate{}, cfg.CurrencyTable)
	oldestDate, err := bqClient.GetOldestDateFromCostItems(ctx)
	if err != nil {
		log.Errorf(err, "failed to get currency dates")
	}
	rates := []bigquery.CurrencyRate{}
	last := false
	for {
		endDate := oldestDate.Add(time.Hour * 24 * 365)
		if oldestDate.Add(time.Hour * 24 * 365).After(time.Now()) {
			endDate = time.Now()
			last = true
		}

		resp, err := currencyClient.RatesPeriod(ctx, "USD", "EUR,NOK", oldestDate, endDate)
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
		oldestDate = oldestDate.Add(time.Hour * 24 * 365)
	}
	for _, rate := range rates {
		fmt.Printf("%#v\n", rate)
		err := bqClient.InsertCurrencyRate(ctx, rate)
		if err != nil {
			log.Errorf(err, "failed to insert currency rate")
			os.Exit(1)
		}
	}
}
