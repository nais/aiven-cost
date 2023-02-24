package currency

import (
	"cloud.google.com/go/bigquery"
)

type CurrencyResponse struct {
	Data struct {
		DataSets []struct {
			Series map[string]struct {
				Observations map[string][]string `json:"observations"`
			} `json:"series"`
		} `json:"dataSets"`

		Structure struct {
			Dimensions struct {
				Series []struct {
					ID     string `json:"id"`
					Values []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"values"`
				} `json:"series"`
				Observation []struct {
					Values []struct {
						ID string `json:"id"`
					} `json:"values"`
				} `json:"observation"`
			} `json:"dimensions"`
		} `json:"structure"`
	} `json:"data"`
}

type CurrencyRate struct {
	TimePeriod string
	USDNOK     string
	EURNOK     string
	USDEUR     string
}

// Save() (row map[string]Value, insertID string, err error)
func (c *CurrencyRate) Save() (map[string]bigquery.Value, string, error) {
	return map[string]bigquery.Value{
		"time_period": c.TimePeriod,
		"usdnok":      c.USDNOK,
		"eurnok":      c.EURNOK,
		"usdeur":      c.USDEUR,
	}, "", nil
}
