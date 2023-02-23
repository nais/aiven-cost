package currency

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
	USD        string
	EUR        string
}
