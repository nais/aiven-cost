package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	client   *http.Client
	apiURL   string
	apiToken string
}

func New(apiToken string) *Client {
	return &Client{
		client:   http.DefaultClient,
		apiURL:   "https://api.apilayer.com/exchangerates_data",
		apiToken: apiToken,
	}
}

func (c *Client) do(ctx context.Context, v any, method, path string, body io.Reader) error {
	fmt.Println(path)
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.apiToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if method == http.MethodGet && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) RatesPeriod(ctx context.Context, base, currency string, start, end time.Time) (CurrencyRateTimeSeriesResponse, error) {
	ret := CurrencyRateTimeSeriesResponse{}

	if err := c.do(ctx, &ret, http.MethodGet, c.apiURL+"/timeseries?start_date="+start.Format("2006-01-02")+"&end_date="+end.Format("2006-01-02")+"&base="+base+"&symbols="+currency, nil); err != nil {
		return CurrencyRateTimeSeriesResponse{}, err
	}

	return ret, nil
}
