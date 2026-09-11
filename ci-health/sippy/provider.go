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
	mu     sync.RWMutex
	data   *HealthSnapshot
	status CollectionStatus
}

type CollectionStatus struct {
	State         string     `json:"state"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Completed     int        `json:"completed"`
	Total         int        `json:"total"`
	Failed        int        `json:"failed"`
	LastCompleted string     `json:"last_completed,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	Error         string     `json:"error,omitempty"`
	HasData       bool       `json:"has_data"`
}

func NewProvider(ctx context.Context, client *Client, catalog *jobs.Catalog, interval time.Duration) *Provider {
	p := &Provider{}

	refresh := func() {
		startedAt := time.Now().UTC()
		fmt.Fprintf(logWriter, "sippy: collecting data...\n")
		p.mu.Lock()
		p.status = CollectionStatus{
			State:     "collecting",
			StartedAt: startedAt,
			Total:     collectionRequestCount(catalog),
			HasData:   p.data != nil,
		}
		p.mu.Unlock()

		progress := func(label string, err error) {
			p.mu.Lock()
			p.status.Completed++
			p.status.LastCompleted = label
			if err != nil {
				p.status.Failed++
				p.status.LastError = err.Error()
			}
			completed, total := p.status.Completed, p.status.Total
			p.mu.Unlock()
			fmt.Fprintf(logWriter, "sippy: collection progress: %d/%d %s\n", completed, total, label)
		}

		snapshot, err := collect(ctx, client, catalog, progress)
		finishedAt := time.Now().UTC()
		if err != nil {
			p.mu.Lock()
			p.status.State = "error"
			p.status.Error = err.Error()
			p.status.FinishedAt = &finishedAt
			p.status.HasData = p.data != nil
			p.mu.Unlock()
			fmt.Fprintf(logWriter, "sippy: collection error after %s: %v\n", finishedAt.Sub(startedAt).Round(time.Millisecond), err)
			return
		}
		p.mu.Lock()
		p.data = snapshot
		p.status.State = "ready"
		p.status.FinishedAt = &finishedAt
		p.status.HasData = true
		p.mu.Unlock()
		fmt.Fprintf(logWriter, "sippy: collection complete in %s: %d presubmits, %d periodics\n", finishedAt.Sub(startedAt).Round(time.Millisecond), len(catalog.BlockingJobs), catalog.PeriodicJobCount())
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

func (p *Provider) Status() CollectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func collectionRequestCount(catalog *jobs.Catalog) int {
	total := 1 + len(catalog.PresubmitProwJobNames())
	for _, names := range catalog.PeriodicProwJobNamesByRelease() {
		total += len(names)
	}
	return total
}

func collect(ctx context.Context, client *Client, catalog *jobs.Catalog, progress func(string, error)) (*HealthSnapshot, error) {
	releases := catalog.Releases()
	presubmitNames := catalog.PresubmitProwJobNames()
	periodicsByRelease := catalog.PeriodicProwJobNamesByRelease()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	analysisEnd := today.AddDate(0, 0, 1)
	analysisBoundary := analysisEnd.AddDate(0, 0, -30)
	analysisStart := analysisBoundary.AddDate(0, 0, -30)

	var mu sync.Mutex
	analyses := make(map[string]*SippyJobAnalysisResponse)
	var recentFailures []SippyTestFailure
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	report := func(label string, err error) {
		if progress != nil {
			progress(label, err)
		}
	}

	var wg sync.WaitGroup

	// Fetch recent failures
	wg.Add(1)
	go func() {
		defer wg.Done()
		label := "recent failures"
		result, err := client.FetchRecentFailures(ctx, "Presubmits", "4h", "24h", 50)
		if err != nil {
			err = fmt.Errorf("recent failures: %w", err)
			setErr(err)
			report(label, err)
			return
		}
		mu.Lock()
		recentFailures = result
		mu.Unlock()
		report(label, nil)
	}()

	analysisSlots := make(chan struct{}, 6)
	fetchAnalysis := func(release, name string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			label := fmt.Sprintf("job analysis %s %s", release, name)
			select {
			case analysisSlots <- struct{}{}:
				defer func() { <-analysisSlots }()
			case <-ctx.Done():
				err := ctx.Err()
				setErr(err)
				report(label, err)
				return
			}
			started := time.Now()
			result, err := client.FetchJobAnalysis(ctx, release, name, analysisStart, analysisBoundary, analysisEnd)
			if err != nil {
				err = fmt.Errorf("job analysis %s/%s: %w", release, name, err)
				setErr(err)
				report(label, err)
				return
			}
			mu.Lock()
			analyses[name] = result
			mu.Unlock()
			fmt.Fprintf(logWriter, "sippy: analysis complete: release=%s job=%s periods=%d duration=%s\n", release, name, len(result.ByPeriod), time.Since(started).Round(time.Millisecond))
			report(label, nil)
		}()
	}

	for _, name := range presubmitNames {
		fetchAnalysis("Presubmits", name)
	}
	for _, release := range releases {
		for _, name := range periodicsByRelease[release] {
			fetchAnalysis(release, name)
		}
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	// Organize fetched data
	raw := &rawData{
		analyses:       analyses,
		recentFailures: recentFailures,
	}

	now := time.Now().UTC()
	snapshot := &HealthSnapshot{
		GeneratedAt: now,
		Windows:     make(map[string]*WindowData),
		Platforms:   catalog.Platforms(),
	}

	for windowKey := range WindowConfigs {
		snapshot.Windows[windowKey] = transformWindow(raw, windowKey, now, catalog)
	}

	return snapshot, nil
}
