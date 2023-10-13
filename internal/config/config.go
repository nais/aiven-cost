package config

import (
	"github.com/kelseyhightower/envconfig"
)

// Aiven configuration
type Aiven struct {
	// ApiHost is the hostname of the Aiven API
	ApiHost string `envconfig:"AIVEN_API_HOST" default:"api.aiven.io"`

	// Token is the Aiven API token
	Token string `envconfig:"AIVEN_API_TOKEN" default:""`
}

// Log configuration
type Log struct {
	// LogFormat Customize the log format. Can be "text" or "json".
	Format string `envconfig:"LOG_FORMAT" default:"text"`

	// LogLevel The log level used in teams-backend.
	Level string `envconfig:"LOG_LEVEL" default:"INFO"`
}

// BigQuery configuration
type BigQuery struct {
	// CostItemsTable is the name of the table containing cost items
	CostItemsTable string `envconfig:"COST_ITEMS_TABLE" default:"cost_items"`

	// CurrencyTable is the name of the table containing currency rates
	CurrencyTable string `envconfig:"CURRENCY_TABLE" default:"currency_rates"`

	// Dataset is the name of the dataset containing the tables
	Dataset string `envconfig:"BIGQUERY_DATASET" default:"aiven_cost_regional"`

	// ProjectID is the name of the project containing the dataset
	ProjectID string `envconfig:"PROJECT_ID" default:"nais-io"`
}

type Currency struct {
	// CurrencyToken is the Currency API token
	Token string `envconfig:"CURRENCY_TOKEN" default:""`
}

// Config is the configuration for the application
type Config struct {
	Aiven    Aiven
	BigQuery BigQuery
	Currency Currency
	Log      Log
}

func New() (*Config, error) {
	cfg := &Config{}

	err := envconfig.Process("", cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
