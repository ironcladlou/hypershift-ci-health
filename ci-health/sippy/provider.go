package sippy

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
)

var logWriter io.Writer = os.Stderr

type Provider struct {
	mu   sync.RWMutex
	data *HealthSnapshot
}

func NewProvider(ctx context.Context, client *Client, interval time.Duration) *Provider {
	p := &Provider{}

	refresh := func() {
		fmt.Fprintf(logWriter, "sippy: collecting data...\n")
		snapshot, err := collect(ctx, client)
		if err != nil {
			fmt.Fprintf(logWriter, "sippy: collection error: %v\n", err)
			return
		}
		p.mu.Lock()
		p.data = snapshot
		p.mu.Unlock()
		fmt.Fprintf(logWriter, "sippy: collection complete: %d jobs\n", len(jobs.BlockingJobs))
	}

	go refresh()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	return p
}

func (p *Provider) Data() *HealthSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.data
}

func collect(ctx context.Context, client *Client) (*HealthSnapshot, error) {
	releases := jobs.Releases()
	presubmitNames := jobs.PresubmitProwJobNames()
	periodicsByRelease := jobs.PeriodicProwJobNamesByRelease()

	type jobsResult struct {
		windowKey string
		release   string
		jobs      []SippyJob
	}
	type runsResult struct {
		release string
		runs    []SippyJobRun
	}

	var mu sync.Mutex
	var jobsResults []jobsResult
	var presubmitRuns []SippyJobRun
	var periodicRunResults []runsResult
	var recentFailures []SippyTestFailure
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	var wg sync.WaitGroup

	// Fetch presubmit jobs for each window
	for _, windowKey := range []string{"7d", "2d"} {
		wg.Add(1)
		go func(wk string) {
			defer wg.Done()
			period := ""
			if wk == "2d" {
				period = "twoDay"
			}
			result, err := client.FetchJobs(ctx, "Presubmits", presubmitNames, period)
			if err != nil {
				setErr(fmt.Errorf("presubmit jobs %s: %w", wk, err))
				return
			}
			mu.Lock()
			jobsResults = append(jobsResults, jobsResult{windowKey: wk, release: "Presubmits", jobs: result})
			mu.Unlock()
		}(windowKey)
	}

	// Fetch periodic jobs for each release and window
	for _, rel := range releases {
		periodicNames := periodicsByRelease[rel]
		for _, windowKey := range []string{"7d", "2d"} {
			wg.Add(1)
			go func(r, wk string, names []string) {
				defer wg.Done()
				period := ""
				if wk == "2d" {
					period = "twoDay"
				}
				result, err := client.FetchJobs(ctx, r, names, period)
				if err != nil {
					setErr(fmt.Errorf("periodic jobs %s/%s: %w", r, wk, err))
					return
				}
				mu.Lock()
				jobsResults = append(jobsResults, jobsResult{windowKey: wk, release: r, jobs: result})
				mu.Unlock()
			}(rel, windowKey, periodicNames)
		}
	}

	// Fetch recent failures
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := client.FetchRecentFailures(ctx, "Presubmits", "4h", "24h", 50)
		if err != nil {
			setErr(fmt.Errorf("recent failures: %w", err))
			return
		}
		mu.Lock()
		recentFailures = result
		mu.Unlock()
	}()

	// Fetch presubmit job runs
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := client.FetchJobRuns(ctx, "Presubmits", presubmitNames, 1500)
		if err != nil {
			setErr(fmt.Errorf("presubmit runs: %w", err))
			return
		}
		mu.Lock()
		presubmitRuns = result
		mu.Unlock()
	}()

	// Fetch periodic job runs per release
	for _, rel := range releases {
		periodicNames := periodicsByRelease[rel]
		wg.Add(1)
		go func(r string, names []string) {
			defer wg.Done()
			result, err := client.FetchJobRuns(ctx, r, names, 1500)
			if err != nil {
				setErr(fmt.Errorf("periodic runs %s: %w", r, err))
				return
			}
			mu.Lock()
			periodicRunResults = append(periodicRunResults, runsResult{release: r, runs: result})
			mu.Unlock()
		}(rel, periodicNames)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	// Organize fetched data
	raw := &rawData{
		presubmitJobs:  make(map[string][]SippyJob),
		periodicJobs:   make(map[string][]SippyJob),
		recentFailures: recentFailures,
		presubmitRuns:  presubmitRuns,
	}

	for _, jr := range jobsResults {
		if jr.release == "Presubmits" {
			raw.presubmitJobs[jr.windowKey] = jr.jobs
		} else {
			raw.periodicJobs[jr.windowKey] = append(raw.periodicJobs[jr.windowKey], jr.jobs...)
		}
	}

	for _, rr := range periodicRunResults {
		raw.periodicRuns = append(raw.periodicRuns, rr.runs...)
	}

	now := time.Now().UTC()
	snapshot := &HealthSnapshot{
		GeneratedAt: now,
		Windows:     make(map[string]*WindowData),
		Platforms:   jobs.Platforms(),
		Versions:    jobs.Releases(),
	}

	for windowKey := range WindowConfigs {
		snapshot.Windows[windowKey] = transformWindow(raw, windowKey, now)
	}

	return snapshot, nil
}
