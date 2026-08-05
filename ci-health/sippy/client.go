package sippy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const DefaultBaseURL = "https://sippy.dptools.openshift.org"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTPClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

type filterItem struct {
	ColumnField   string `json:"columnField"`
	OperatorValue string `json:"operatorValue"`
	Value         string `json:"value"`
}

type filter struct {
	Items        []filterItem `json:"items"`
	LinkOperator string       `json:"linkOperator"`
}

func exactMatchFilter(field string, values []string) string {
	items := make([]filterItem, len(values))
	for i, v := range values {
		items[i] = filterItem{ColumnField: field, OperatorValue: "equals", Value: v}
	}
	b, _ := json.Marshal(filter{Items: items, LinkOperator: "or"})
	return string(b)
}

func (c *Client) get(ctx context.Context, path string, params url.Values) (*http.Response, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("sippy %s: %s", path, resp.Status)
	}
	return resp, nil
}

// FetchJobs fetches job stats filtered to the exact job names provided.
func (c *Client) FetchJobs(ctx context.Context, release string, jobNames []string, period string) ([]SippyJob, error) {
	params := url.Values{
		"release": {release},
		"filter":  {exactMatchFilter("name", jobNames)},
	}
	if period != "" {
		params.Set("period", period)
	}
	resp, err := c.get(ctx, "/api/jobs", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jobs []SippyJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("decoding jobs: %w", err)
	}
	return jobs, nil
}

// FetchJobRuns fetches job run data filtered to the exact job names provided.
func (c *Client) FetchJobRuns(ctx context.Context, release string, jobNames []string, perPage int) ([]SippyJobRun, error) {
	params := url.Values{
		"release":   {release},
		"filter":    {exactMatchFilter("job", jobNames)},
		"perPage":   {strconv.Itoa(perPage)},
		"sortField": {"timestamp"},
		"sort":      {"desc"},
	}
	resp, err := c.get(ctx, "/api/jobs/runs", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result SippyJobRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding job runs: %w", err)
	}
	return result.Rows, nil
}

func (c *Client) FetchRecentFailures(ctx context.Context, release, period, previousPeriod string, perPage int) ([]SippyTestFailure, error) {
	params := url.Values{
		"release":        {release},
		"period":         {period},
		"previousPeriod": {previousPeriod},
		"perPage":        {strconv.Itoa(perPage)},
		"includeOutputs": {"true"},
	}
	resp, err := c.get(ctx, "/api/tests/recent_failures", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result SippyRecentFailuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding recent failures: %w", err)
	}
	return result.Rows, nil
}
