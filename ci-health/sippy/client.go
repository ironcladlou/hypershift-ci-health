package sippy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const DefaultBaseURL = "https://sippy.dptools.openshift.org"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: http.DefaultClient,
	}
}

func jobFilter(value string) string {
	return fmt.Sprintf(`{"items":[{"columnField":"name","operatorValue":"contains","value":%q}],"linkOperator":"and"}`, value)
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

func (c *Client) FetchJobs(ctx context.Context, release, filterValue, period string) ([]SippyJob, error) {
	params := url.Values{
		"release": {release},
		"filter":  {jobFilter(filterValue)},
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

func (c *Client) FetchJobRuns(ctx context.Context, release, filterValue string, perPage int) ([]SippyJobRun, error) {
	params := url.Values{
		"release":   {release},
		"filter":    {jobFilter(filterValue)},
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
