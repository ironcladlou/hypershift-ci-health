package sippy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://sippy.dptools.openshift.org"

const (
	maxRequestAttempts = 4
	initialRetryDelay  = time.Second
)

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

func fieldFilter(field, operator, value string) string {
	b, _ := json.Marshal(filter{
		Items:        []filterItem{{ColumnField: field, OperatorValue: operator, Value: value}},
		LinkOperator: "and",
	})
	return string(b)
}

func (c *Client) get(ctx context.Context, path string, params url.Values) (response *http.Response, err error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path
	u.RawQuery = params.Encode()
	defer func() {
		if err != nil {
			fmt.Fprintf(logWriter, "sippy: request failed: url=%s error=%v\n", u.String(), err)
		}
	}()

	delay := initialRetryDelay
	for attempt := 1; attempt <= maxRequestAttempts; attempt++ {
		attemptStarted := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, requestErr := c.HTTPClient.Do(req)
		if requestErr == nil && resp.StatusCode == http.StatusOK {
			if attempt > 1 {
				fmt.Fprintf(logWriter, "sippy: request recovered: path=%s url=%s release=%s attempt=%d/%d duration=%s\n", path, u.String(), params.Get("release"), attempt, maxRequestAttempts, time.Since(attemptStarted).Round(time.Millisecond))
			}
			return resp, nil
		}

		retryable := requestErr != nil
		if resp != nil {
			resp.Body.Close()
			retryable = retryableStatus(resp.StatusCode)
			requestErr = fmt.Errorf("sippy %s: %s", path, resp.Status)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !retryable || attempt == maxRequestAttempts {
			if attempt == 1 {
				return nil, requestErr
			}
			return nil, fmt.Errorf("%w after %d attempts", requestErr, attempt)
		}
		fmt.Fprintf(logWriter, "sippy: request retry: path=%s url=%s release=%s attempt=%d/%d duration=%s error=%v backoff=%s\n", path, u.String(), params.Get("release"), attempt, maxRequestAttempts, time.Since(attemptStarted).Round(time.Millisecond), requestErr, delay)
		if err := waitForRetry(ctx, delay); err != nil {
			return nil, err
		}
		delay *= 2
	}
	return nil, fmt.Errorf("sippy %s: request failed", path)
}

func decodeResponse(resp *http.Response, result any, description string) error {
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		requestURL := ""
		if resp.Request != nil && resp.Request.URL != nil {
			requestURL = resp.Request.URL.String()
		}
		err = fmt.Errorf("decoding %s: %w", description, err)
		fmt.Fprintf(logWriter, "sippy: response decode failed: url=%s error=%v\n", requestURL, err)
		return err
	}
	return nil
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) fetchJobAnalysis(ctx context.Context, release, jobName string, start, boundary, end time.Time) (*jobAnalysisResponse, error) {
	params := url.Values{
		"release":  {release},
		"filter":   {exactMatchFilter("name", []string{jobName})},
		"period":   {"hour"},
		"start":    {start.Format(time.DateOnly)},
		"boundary": {boundary.Format(time.DateOnly)},
		"end":      {end.Format(time.DateOnly)},
	}
	resp, err := c.get(ctx, "/api/jobs/analysis", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result jobAnalysisResponse
	if err := decodeResponse(resp, &result, "job analysis"); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) fetchComponentReadinessMembership(ctx context.Context, release string) ([]ComponentReadinessMembership, error) {
	params := url.Values{
		"release": {release},
		"filter":  {fieldFilter("name", "starts with", "periodic-ci-openshift-hypershift-")},
	}
	resp, err := c.get(ctx, "/api/jobs", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []jobListItem
	if err := decodeResponse(resp, &result, "component readiness jobs"); err != nil {
		return nil, err
	}

	memberships := make([]ComponentReadinessMembership, 0, len(result))
	for _, item := range result {
		var tier JobTier
		for _, variant := range item.Variants {
			value, found := strings.CutPrefix(variant, "JobTier:")
			if !found {
				continue
			}
			candidate := JobTier(value)
			if tier != "" && tier != candidate {
				return nil, fmt.Errorf("job %q has conflicting JobTier variants %q and %q", item.Name, tier, candidate)
			}
			tier = candidate
		}
		if tier == "" {
			continue
		}
		memberships = append(memberships, ComponentReadinessMembership{
			Release:     release,
			ProwJobName: item.Name,
			Tier:        tier,
		})
	}
	return memberships, nil
}

func (c *Client) fetchRecentFailures(ctx context.Context, release, period, previousPeriod string, perPage int) ([]RecentFailure, error) {
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

	var result recentFailuresResponse
	if err := decodeResponse(resp, &result, "recent failures"); err != nil {
		return nil, err
	}
	return result.Rows, nil
}
