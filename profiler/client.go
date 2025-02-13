package profiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// maxConcurrency is the maximum number of concurrent requests to make to the
// Datadog API.
const maxConcurrency = 5

// Client is a client for the Datadog API.
type Client struct {
	site        string
	apiKey      string
	appKey      string
	concurrency chan struct{}
}

// NewClient creates a new Datadog API client.
// It takes the API key, application key, and site as arguments.
// It returns an error if any of the required keys are missing.
// Example:
//
//	client, err := datadogpgo.NewClient("your_api_key", "your_app_key", "datadoghq.com")
func NewClient(apiKey, appKey, site string) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("DataDog API key is required")
	}
	if appKey == "" {
		return nil, errors.New("DataDog Application key is required")
	}
	if site == "" {
		site = "datadoghq.com"
	}

	return &Client{
		apiKey:      apiKey,
		appKey:      appKey,
		site:        site,
		concurrency: make(chan struct{}, maxConcurrency),
	}, nil
}

// ClientFromEnv creates a new Datadog client from environment variables.
// It reads DD_API_KEY, DD_APP_KEY and DD_SITE, it returns an error if API Key, App key is missing.
// DD_SITE has default value datadoghq.com
func ClientFromEnv() (*Client, error) {
	return NewClient(os.Getenv("DD_API_KEY"), os.Getenv("DD_APP_KEY"), os.Getenv("DD_SITE"))
}

// GetCPUProfile retrieves a merged CPU profile for the specified service and environment.
// It automatically adds the "service" and "env" tags to the query.
// It uses the PGO endpoint if runtime is go
// It handles profile merging and returns an io.Reader for the resulting pprof file.
//
// Example:
//
//	profileReader, err := client.GetCPUProfile(ctx, "my-service", "prod", "go", 3*time.Hour, 5)
//	if err != nil {
//		// Handle error
//	}
//	defer profileReader.Close()
//
// // Use profileReader to read the pprof data...
func (c *Client) GetCPUProfile(ctx context.Context, service, environment, runtime string, window time.Duration, limit int) (io.Reader, error) {
	query := fmt.Sprintf("service:%s env:%s", service, environment)
	// if runtime != "" && !strings.Contains(query, "runtime:") && !strings.Contains(query, "language:") {
	// 	query += fmt.Sprintf(" runtime:%s", runtime)
	// }

	queries := []SearchQuery{
		{
			Filter: SearchFilter{
				From:  JSONTime{time.Now().Add(-window)},
				To:    JSONTime{time.Now()},
				Query: query,
			},
			Sort: SearchSort{
				Order: "desc",
				// TODO(fg) or use @metrics.core_cpu_time_total?
				Field: "@metrics.core_cpu_cores",
			},
			Limit: limit,
		},
	}

	// Set limit to 1 to get only top profile
	queries[0].Limit = 1

	// Search for the top profile
	profiles, err := c.SearchProfiles(ctx, queries[0])
	if err != nil {
		return nil, err
	}

	// Download the profile
	download, err := c.DownloadProfile(ctx, profiles[0])
	if err != nil {
		return nil, err
	}

	// Extract CPU profile data
	cpuData, err := download.ExtractCPUProfile()
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(cpuData), nil

	// // if err := ApplyNoInlineHack(prof); err != nil {
	// // 	return nil, err
	// // }

	// // Create pipe to write profile data
	// pr, pw := io.Pipe()
	// go func() {
	// 	defer pw.Close()
	// 	err := prof.Write(pw)
	// 	if err != nil {
	// 		pw.CloseWithError(err)
	// 		return
	// 	}
	// }()

	// return pr, nil
}

// SearchAndDownloadProfiles searches for profiles using the given queries and
// downloads them.
func (c *Client) SearchAndDownloadProfiles(ctx context.Context, queries []SearchQuery) (profiles *ProfilesDownload, err error) {
	defer wrapErr(&err, "search and download profiles")
	defer c.limitConcurrency()()

	var payload = struct {
		Queries []SearchQuery `json:"queries"`
	}{queries}

	data, err := c.post(ctx, "/api/unstable/profiles/gopgo", payload)
	if err != nil {
		return nil, err
	}
	return &ProfilesDownload{data: data}, nil
}

// SearchProfiles searches for profiles using the given query. It returns a list
// of profiles and an error if any.
func (c *Client) SearchProfiles(ctx context.Context, query SearchQuery) (profiles []*SearchProfile, err error) {
	defer wrapErr(&err, "search profiles")
	defer c.limitConcurrency()()
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				ID            string   `json:"id"`
				Service       string   `json:"service"`
				DurationNanos float64  `json:"duration_nanos"`
				Timestamp     JSONTime `json:"timestamp"`
				Custom        struct {
					Metrics struct {
						CoreCPUCores float64 `json:"core_cpu_cores"`
					} `json:"metrics"`
				} `json:"custom"`
			} `json:"attributes"`
		} `json:"data"`
	}
	data, err := c.post(ctx, "/api/unstable/profiles/list", query)
	if err != nil {
		return nil, err
	} else if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.Data) == 0 {
		return nil, errors.New("no profiles found")
	}

	for _, item := range response.Data {
		p := &SearchProfile{
			EventID:   item.ID,
			ProfileID: item.Attributes.ID,
			Service:   item.Attributes.Service,
			CPUCores:  item.Attributes.Custom.Metrics.CoreCPUCores,
			Timestamp: item.Attributes.Timestamp.Time,
			Duration:  time.Duration(item.Attributes.DurationNanos),
		}
		profiles = append(profiles, p)
	}
	return
}

// DownloadProfile downloads the profile identified by the given SearchProfile.
func (c *Client) DownloadProfile(ctx context.Context, p *SearchProfile) (d ProfileDownload, err error) {
	defer wrapErr(&err, "download profile")
	defer c.limitConcurrency()()
	req, err := c.request(ctx, "GET", fmt.Sprintf("/api/ui/profiling/profiles/%s/download?eventId=%s", p.ProfileID, p.EventID), nil)
	if err != nil {
		return ProfileDownload{}, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProfileDownload{}, err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return ProfileDownload{}, err
	}
	return ProfileDownload{data: data}, nil
}

// request creates a new HTTP request with the given method and path and sets
// the required headers.
func (c *Client) request(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := fmt.Sprintf("https://app.%s%s", c.site, path)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "datadog-pgo/"+version)
	req.Header.Set("DD-APPLICATION-KEY", c.appKey)
	req.Header.Set("DD-API-KEY", c.apiKey)
	return req, nil
}

// post sends a POST request to the given path with the given payload and decodes
// the response.
func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := c.request(ctx, "POST", path, reqBody)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("(path:%s) %s: please check that your DD_API_KEY, DD_APP_KEY and DD_SITE env vars are set correctly and that your account has profiles matching your query", path, res.Status)
	}
	return resBody, nil
}

// limitConcurrency blocks until a slot is available in the concurrency channel.
// It returns a function that should be called to release the slot.
func (c *Client) limitConcurrency() func() {
	c.concurrency <- struct{}{}
	return func() { <-c.concurrency }
}
