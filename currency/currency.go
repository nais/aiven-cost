package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	client *http.Client
	apiURL string
}

func New() *Client {
	return &Client{
		client: http.DefaultClient,
		apiURL: "https://data.norges-bank.no/api/data/EXR/",
	}
}

func (c *Client) do(ctx context.Context, v any, method, path string, body io.Reader) error {
	fmt.Println(path)
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

	if err := c.do(ctx, &ret, http.MethodGet, c.apiURL+"M."+currency+".NOK.SP?startPeriod="+year+"-"+month+"&format=sdmx-json&locale=no", nil); err != nil {
		return CurrencyResponse{}, err
	}

	return ret, nil
}

func (c *Client) getRateEstimateForCurrentMonth(ctx context.Context, currency string) (CurrencyRate, error) {
	t := time.Now()
	year := fmt.Sprintf("%d", t.Year())
	month := fmt.Sprintf("%02d", t.Month())
	day := fmt.Sprintf("%02d", t.Day())

	ret := CurrencyResponse{}

	if err := c.do(ctx, &ret, http.MethodGet, c.apiURL+"B."+currency+".NOK.SP?startPeriod="+year+"-"+month+"-01&endPeriod="+year+"-"+month+"-"+day+"&format=sdmx-json&locale=no", nil); err != nil {
		return CurrencyRate{}, err
	}

	eur_key := 0
	usd_key := 0
	for i := range ret.Data.Structure.Dimensions.Series {
		if ret.Data.Structure.Dimensions.Series[i].ID == "BASE_CUR" {
			for j := range ret.Data.Structure.Dimensions.Series[i].Values {
				if ret.Data.Structure.Dimensions.Series[i].Values[j].ID == "EUR" {
					eur_key = j
				}
				if ret.Data.Structure.Dimensions.Series[i].Values[j].ID == "USD" {
					usd_key = j
				}
			}
		}
	}

	eur_sum := 0.0
	usd_sum := 0.0
	number_of_samples := len(ret.Data.Structure.Dimensions.Observation[0].Values)

	for id := range ret.Data.Structure.Dimensions.Observation[0].Values {
		eur := ret.Data.DataSets[0].Series["0:"+strconv.Itoa(eur_key)+":0:0"].Observations[strconv.Itoa(id)][0]
		eur_sum += toFloat(eur)
		usd := ret.Data.DataSets[0].Series["0:"+strconv.Itoa(usd_key)+":0:0"].Observations[strconv.Itoa(id)][0]
		usd_sum += toFloat(usd)
	}

	return CurrencyRate{
		TimePeriod: year + "-" + month,
		EURNOK:        fmt.Sprintf("%f", eur_sum/float64(number_of_samples)),
		USDNOK:        fmt.Sprintf("%f", usd_sum/float64(number_of_samples)),
		USDEUR:        fmt.Sprintf("%f", (usd_sum/float64(number_of_samples))/(eur_sum/float64(number_of_samples))),
	}, nil
}

func toFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func (c *Client) GetRates(ctx context.Context) ([]CurrencyRate, error) {
	ret := []CurrencyRate{}

	rates, err := c.getRates(ctx, "EUR+USD", "2017", "01")
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
		eurnok := rates.Data.DataSets[0].Series["0:"+strconv.Itoa(eur_key)+":0:0"].Observations[strconv.Itoa(id)][0]
		usdnok := rates.Data.DataSets[0].Series["0:"+strconv.Itoa(usd_key)+":0:0"].Observations[strconv.Itoa(id)][0]
		ret = append(ret, CurrencyRate{
			TimePeriod: value.ID,
			EURNOK: eurnok,
			USDNOK: usdnok,
			USDEUR: fmt.Sprintf("%f", toFloat(usdnok)/toFloat(eurnok)),
		})
	}
	est, err := c.getRateEstimateForCurrentMonth(ctx, "EUR+USD")
	if err != nil {
		return nil, err
	}
	ret = append(ret, est)

	return ret, nil
}
