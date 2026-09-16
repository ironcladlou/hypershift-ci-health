package sippy

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
)

var logWriter io.Writer = os.Stderr

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

// CollectSnapshot collects one immutable health snapshot for a catalog.
func CollectSnapshot(ctx context.Context, client *Client, catalog *jobs.Catalog) (*HealthSnapshot, CollectionStatus, error) {
	staticTargets := catalog.AnalysisTargets()
	startedAt := time.Now().UTC()
	fmt.Fprintf(logWriter, "sippy: collecting data...\n")
	status := CollectionStatus{
		State:     "collecting",
		StartedAt: startedAt,
		Total:     len(catalog.Releases()),
	}
	var statusMu sync.Mutex
	progress := func(label string, err error) {
		statusMu.Lock()
		status.Completed++
		status.LastCompleted = label
		if err != nil {
			status.Failed++
			status.LastError = err.Error()
		}
		completed, total := status.Completed, status.Total
		statusMu.Unlock()
		fmt.Fprintf(logWriter, "sippy: collection progress: %d/%d %s\n", completed, total, label)
	}
	addTotal := func(count int) {
		statusMu.Lock()
		status.Total += count
		statusMu.Unlock()
	}

	snapshot, err := collect(ctx, client, catalog, staticTargets, progress, addTotal)
	finishedAt := time.Now().UTC()
	statusMu.Lock()
	status.FinishedAt = &finishedAt
	if err != nil {
		status.State = "error"
		status.Error = err.Error()
		statusMu.Unlock()
		fmt.Fprintf(logWriter, "sippy: collection error after %s: %v\n", finishedAt.Sub(startedAt).Round(time.Millisecond), err)
		return nil, status, err
	}
	status.State = "ready"
	status.HasData = true
	statusMu.Unlock()
	componentReadinessCount := len(snapshot.Windows["1w"].ComponentReadinessJobs)
	fmt.Fprintf(logWriter, "sippy: collection complete in %s: %d presubmits displayed, %d static analysis targets, %d component readiness blockers\n", finishedAt.Sub(startedAt).Round(time.Millisecond), len(catalog.BlockingJobs), len(staticTargets), componentReadinessCount)
	return snapshot, status, nil
}

func collect(ctx context.Context, client *Client, catalog *jobs.Catalog, staticTargets []jobs.AnalysisTarget, progress func(string, error), addTotal func(int)) (*HealthSnapshot, error) {
	releases := catalog.Releases()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	analysisEnd := today.AddDate(0, 0, 1)
	analysisBoundary := analysisEnd.AddDate(0, 0, -30)
	analysisStart := analysisBoundary.AddDate(0, 0, -30)

	var mu sync.Mutex
	analyses := make(map[analysisKey]*SippyJobAnalysisResponse)
	var recentFailures []SippyTestFailure
	var memberships []jobs.ComponentReadinessMembership
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
	discoverySlots := make(chan struct{}, 6)
	for _, release := range releases {
		release := release
		wg.Add(1)
		go func() {
			defer wg.Done()
			label := fmt.Sprintf("component readiness jobs %s", release)
			select {
			case discoverySlots <- struct{}{}:
				defer func() { <-discoverySlots }()
			case <-ctx.Done():
				err := ctx.Err()
				setErr(err)
				report(label, err)
				return
			}
			result, err := client.FetchComponentReadinessMembership(ctx, release)
			if err != nil {
				err = fmt.Errorf("component readiness jobs %s: %w", release, err)
				setErr(err)
				report(label, err)
				return
			}
			mu.Lock()
			memberships = append(memberships, result...)
			mu.Unlock()
			report(label, nil)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	componentReadinessJobs := catalog.ComponentReadinessJobs(memberships)
	type analysisRequest struct {
		key  analysisKey
		name string
	}
	requestSet := make(map[analysisKey]analysisRequest)
	for _, target := range staticTargets {
		key := analysisKey{release: target.Release, jobID: target.Job.ID}
		requestSet[key] = analysisRequest{key: key, name: target.Job.Name}
	}
	for _, job := range componentReadinessJobs {
		key := analysisKey{release: job.Membership.Release, jobID: job.Membership.ProwJobName}
		requestSet[key] = analysisRequest{key: key, name: job.Membership.ProwJobName}
	}
	requests := make([]analysisRequest, 0, len(requestSet))
	for _, request := range requestSet {
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].key.release != requests[j].key.release {
			return requests[i].key.release < requests[j].key.release
		}
		return requests[i].key.jobID < requests[j].key.jobID
	})
	if addTotal != nil {
		addTotal(1 + len(requests))
	}

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
	fetchAnalysis := func(request analysisRequest) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, name := request.key.release, request.name
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
			analyses[request.key] = result
			mu.Unlock()
			fmt.Fprintf(logWriter, "sippy: analysis complete: release=%s job=%s periods=%d duration=%s\n", release, name, len(result.ByPeriod), time.Since(started).Round(time.Millisecond))
			report(label, nil)
		}()
	}

	for _, request := range requests {
		fetchAnalysis(request)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	// Organize fetched data
	raw := &rawData{
		analyses:           analyses,
		recentFailures:     recentFailures,
		componentReadiness: componentReadinessJobs,
	}

	now := time.Now().UTC()
	snapshot := &HealthSnapshot{
		GeneratedAt: now,
		Windows:     make(map[string]*WindowData),
		Platforms:   collectedPlatforms(catalog.Platforms(), componentReadinessJobs),
		Releases:    catalog.Releases(),
	}

	for windowKey := range WindowConfigs {
		snapshot.Windows[windowKey] = transformWindow(raw, windowKey, now, catalog)
	}

	return snapshot, nil
}

func collectedPlatforms(static []string, componentReadiness []jobs.ComponentReadinessJobConfig) []string {
	seen := make(map[string]struct{}, len(static))
	for _, platform := range static {
		seen[platform] = struct{}{}
	}
	for _, job := range componentReadiness {
		if job.Job == nil {
			continue
		}
		for _, platform := range job.Job.Platforms {
			seen[platform] = struct{}{}
		}
	}
	platforms := make([]string, 0, len(seen))
	for platform := range seen {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	return platforms
}
