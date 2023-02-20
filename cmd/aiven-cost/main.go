package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/exp/slog"
)

var cfg = struct {
	APIHost    string
	LogLevel   string
	AivenToken string
}{
	APIHost:    "api.aiven.io",
	LogLevel:   "info",
	AivenToken: "",
}

func main() {
	flag.StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "API host")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "which log level to output")
	flag.StringVar(&cfg.AivenToken, "aiven-token", os.Getenv("AIVEN_TOKEN"), "Aiven API token")

	client := &http.Client{}
	resp, err := get(client, "/v1/project")
	if err != nil {
		slog.Error("failed to get projects: %v", err)
		panic("Unable to continue without projects")
	}
	type BillingReport struct {
		InvoiceId   *string
		Environment *string
		Team        *string
		Date        *time.Time
		Service     *string
		CostInEuros *float64
		Tenant      *string
	}

	type projects struct {
		Projects []struct {
			Name         string `json:"project_name"`
			BillingGroup string `json:"billing_group_id"`
		} `json:"projects"`
	}
	projectList := projects{}
	err = json.Unmarshal(resp, &projectList)
	if err != nil {
		slog.Error("failed to unmarshal projects: %v", err)
		panic("Unable to continue without projects")
	}

	type service struct {
		Services []struct {
			Tags struct {
				Tenant string `json:"tenant"`
			} `json:"tags"`
		} `json:"services"`
	}

	for _, project := range projectList.Projects {
		fmt.Printf("%v \n", project)
		res := BillingReport{}
		service := service{}
		resp, err := get(client, fmt.Sprintf("/v1/project/%s/service", project.Name))
		if err != nil {
			slog.Error("failed to get services: %v", err)
			panic("Unable to continue without services")
		}
		err = json.Unmarshal(resp, &service)
		if err != nil {
			slog.Error("failed to unmarshal services: %v", err)
			panic("Unable to continue without services")
		}
		for _, s := range service.Services {
			s := s
			if s.Tags.Tenant != "" {
				res.Tenant = &s.Tags.Tenant
				break
			}
		}
		if res.Tenant == nil {
			log.Printf("Tenant for %q is empty", project.Name)
		}
		fmt.Printf("%v\n", *res.Tenant)
		break
	}
}

func get(client *http.Client, path string) ([]byte, error) {
	req, err := http.NewRequest("GET", "https://"+cfg.APIHost+path, nil)
	if err != nil {
		return []byte{}, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("aivenv1 %s", cfg.AivenToken))

	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}
