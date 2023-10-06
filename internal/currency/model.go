package currency

type CurrencyRateTimeSeriesResponse struct {
	Base    string `json:"base"`
	EndDate string `json:"end_date"`
	Rates   map[string]struct {
		EUR float64 `json:"EUR"`
		NOK float64 `json:"NOK"`
	} `json:"rates"`
	StartDate  string `json:"start_date"`
	Success    bool   `json:"success"`
	Timeseries bool   `json:"timeseries"`
}

type CurrencyRateDailyResponse struct {
	Base       string `json:"base"`
	Date       string `json:"date"`
	Historical bool   `json:"historical"`
	Rates      struct {
		Eur float64 `json:"EUR"`
		Nok float64 `json:"NOK"`
	} `json:"rates"`
	Success   bool `json:"success"`
	Timestamp int  `json:"timestamp"`
}
