package config

type Config struct {
	APIHost        string
	LogLevel       string
	AivenToken     string
	ApiKey         string
	CostItemsTable string
	CurrencyTable  string
	ProjectID      string
	Dataset        string
}

func New() *Config {
	cfg := &Config{
		APIHost:        "api.aiven.io",
		LogLevel:       "info",
		CostItemsTable: "cost_items",
		CurrencyTable:  "currency_rates",
		ProjectID:      "nais-io",
		Dataset:        "aiven_cost",
	}
	return cfg
}
