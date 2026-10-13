package sippy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRequestErrorsLogFullURL(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		recover bool
	}{
		{"non-retryable", http.StatusBadRequest, false},
		{"retryable", http.StatusInternalServerError, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := logWriter
			logWriter = &logs
			t.Cleanup(func() { logWriter = previous })
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if test.recover && attempts > 1 {
					w.Write([]byte(`{}`))
					return
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			params := url.Values{"release": {"Presubmits"}, "filter": {exactMatchFilter("name", []string{"pull-ci-example-job"})}, "start": {"2026-10-01"}}
			client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
			resp, err := client.get(context.Background(), "/api/jobs/analysis", params)
			if resp != nil {
				resp.Body.Close()
			}
			if (err == nil) != test.recover {
				t.Fatalf("unexpected request result: %v", err)
			}
			wantURL := "url=" + server.URL + "/api/jobs/analysis?" + params.Encode()
			if !strings.Contains(logs.String(), wantURL) {
				t.Fatalf("missing complete URL %q in %s", wantURL, logs.String())
			}
			if test.recover && !strings.Contains(logs.String(), "request retry:") {
				t.Fatal("missing retry diagnostic")
			}
			if !test.recover && !strings.Contains(logs.String(), "request failed:") {
				t.Fatal("missing terminal failure diagnostic")
			}
		})
	}
}

func TestDecodeErrorsLogFullURL(t *testing.T) {
	var logs bytes.Buffer
	previous := logWriter
	logWriter = &logs
	t.Cleanup(func() { logWriter = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("invalid JSON")) }))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, err := client.fetchJobAnalysis(context.Background(), "5.1", "example-job", start, start.AddDate(0, 0, 1), start.AddDate(0, 0, 2)); err == nil {
		t.Fatal("invalid response accepted")
	}
	params := url.Values{"release": {"5.1"}, "filter": {exactMatchFilter("name", []string{"example-job"})}, "period": {"hour"}, "start": {"2026-10-01"}, "boundary": {"2026-10-02"}, "end": {"2026-10-03"}}
	if !strings.Contains(logs.String(), "response decode failed: url="+server.URL+"/api/jobs/analysis?"+params.Encode()) {
		t.Fatalf("missing response URL: %s", logs.String())
	}
}
