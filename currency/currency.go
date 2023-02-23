package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type Client struct {
	client *http.Client
	apiURL string
}

func New() *Client {
	return &Client{
		client: http.DefaultClient,
		apiURL: "https://data.norges-bank.no/api/data/EXR/M.",
	}
}

func (c *Client) do(ctx context.Context, v any, method, path string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return err
	}

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

func (c *Client) getRates(ctx context.Context, currency, year, month string) (CurrencyResponse, error) {
	ret := CurrencyResponse{}

	if err := c.do(ctx, &ret, http.MethodGet, c.apiURL+currency+".NOK.SP?startPeriod="+year+"-"+month+"&format=sdmx-json&locale=no", nil); err != nil {
		return CurrencyResponse{}, err
	}

	return ret, nil
}

func (c *Client) GetRates(ctx context.Context) ([]CurrencyRate, error) {
	ret := []CurrencyRate{}

	rates, err := c.getRates(ctx, "EUR+USD", "2020", "01")
	if err != nil {
		return ret, err
	}
	eur_key := 0
	usd_key := 0
	for i := range rates.Data.Structure.Dimensions.Series {
		if rates.Data.Structure.Dimensions.Series[i].ID == "BASE_CUR" {
			for j := range rates.Data.Structure.Dimensions.Series[i].Values {
				if rates.Data.Structure.Dimensions.Series[i].Values[j].ID == "EUR" {
					eur_key = j
				}
				if rates.Data.Structure.Dimensions.Series[i].Values[j].ID == "USD" {
					usd_key = j
				}
			}
		}
	}
	for id, value := range rates.Data.Structure.Dimensions.Observation[0].Values {
		ret = append(ret, CurrencyRate{
			TimePeriod: value.ID,
			EUR:        rates.Data.DataSets[0].Series["0:"+strconv.Itoa(eur_key)+":0:0"].Observations[strconv.Itoa(id)][0],
			USD:        rates.Data.DataSets[0].Series["0:"+strconv.Itoa(usd_key)+":0:0"].Observations[strconv.Itoa(id)][0],
		})
	}

	return ret, nil
}
