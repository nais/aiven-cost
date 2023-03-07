package config

type Config struct {
	AivenAPI       string
	LogLevel       string
	AivenToken     string
	CurrencyToken  string
	CostItemsTable string
	CurrencyTable  string
	ProjectID      string
	Dataset        string
}

func New() *Config {
	cfg := &Config{
		AivenAPI:       "api.aiven.io",
		LogLevel:       "info",
		CostItemsTable: "cost_items",
		CurrencyTable:  "currency_rates",
		ProjectID:      "nais-io",
		Dataset:        "aiven_cost_regional",
	}
	return cfg
}
