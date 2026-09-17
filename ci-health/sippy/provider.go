package sippy

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
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
	HasData       bool       `json:"has_data"`
}

// CollectObservation captures Sippy-owned facts. Individual request failures
// remain visible in the immutable result instead of discarding successful data.
func CollectObservation(ctx context.Context, client *Client, registry *jobregistry.Registry, plan *reportplan.Plan) (*Observation, CollectionStatus, error) {
	if err := plan.Validate(registry); err != nil {
		return nil, CollectionStatus{}, err
	}
	planDigest, err := reportplan.Digest(plan)
	if err != nil {
		return nil, CollectionStatus{}, err
	}
	started := time.Now().UTC()
	status := CollectionStatus{State: "collecting", StartedAt: started}
	observation := &Observation{APIVersion: CurrentObservationAPIVersion, PlanDigest: planDigest, SourceBaseURL: client.BaseURL, Releases: append([]string(nil), plan.Releases...), StartedAt: started, Memberships: []ComponentReadinessMembership{}, Analyses: []JobAnalysis{}, RecentFailures: []RecentFailure{}, Collection: []CollectionResult{}}
	index := registry.Index()
	type request struct{ kind, release, name string }
	var mu sync.Mutex
	record := func(result CollectionResult) {
		mu.Lock()
		defer mu.Unlock()
		observation.Collection = append(observation.Collection, result)
		status.Completed++
		status.LastCompleted = result.Kind
		if result.Error != "" {
			status.Failed++
			status.LastError = result.Error
		}
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 6)
	run := func(requests []request) {
		mu.Lock()
		status.Total += len(requests)
		mu.Unlock()
		for _, req := range requests {
			req := req
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case slots <- struct{}{}:
					defer func() { <-slots }()
				case <-ctx.Done():
					record(CollectionResult{Kind: req.kind, Release: req.release, ProwJobName: req.name, Error: ctx.Err().Error()})
					return
				}
				result := CollectionResult{Kind: req.kind, Release: req.release, ProwJobName: req.name}
				switch req.kind {
				case "membership":
					memberships, fetchErr := client.fetchComponentReadinessMembership(ctx, req.release)
					if fetchErr != nil {
						result.Error = fetchErr.Error()
					} else {
						mu.Lock()
						observation.Memberships = append(observation.Memberships, memberships...)
						mu.Unlock()
					}
				case "recent-failures":
					failures, fetchErr := client.fetchRecentFailures(ctx, "Presubmits", "4h", "24h", 50)
					if fetchErr != nil {
						result.Error = fetchErr.Error()
					} else {
						mu.Lock()
						observation.RecentFailures = failures
						mu.Unlock()
					}
				case "analysis":
					today := started.Truncate(24 * time.Hour)
					end := today.AddDate(0, 0, 1)
					boundary := end.AddDate(0, 0, -30)
					start := boundary.AddDate(0, 0, -30)
					analysis, fetchErr := client.fetchJobAnalysis(ctx, req.release, req.name, start, boundary, end)
					if fetchErr != nil {
						result.Error = fetchErr.Error()
					} else {
						mu.Lock()
						observation.Analyses = append(observation.Analyses, JobAnalysis{Target: AnalysisTarget{Release: req.release, ProwJobName: req.name}, ByPeriod: analysis.ByPeriod})
						mu.Unlock()
					}
				}
				record(result)
			}()
		}
		wg.Wait()
	}
	membershipRequests := make([]request, 0, len(plan.Releases))
	for _, release := range plan.Releases {
		membershipRequests = append(membershipRequests, request{kind: "membership", release: release})
	}
	run(membershipRequests)

	requests := []request{{kind: "recent-failures", release: "Presubmits"}}
	seenAnalyses := map[AnalysisTarget]bool{}
	addAnalysis := func(release, name string) {
		target := AnalysisTarget{Release: release, ProwJobName: name}
		if !seenAnalyses[target] {
			seenAnalyses[target] = true
			requests = append(requests, request{kind: "analysis", release: release, name: name})
		}
	}
	for _, target := range plan.AnalysisTargets {
		addAnalysis(target.Release, index[target.JobID].Name)
	}
	for _, membership := range observation.Memberships {
		if membership.Tier == JobTierStandard {
			addAnalysis(membership.Release, membership.ProwJobName)
		}
	}
	run(requests)
	sort.Slice(observation.Memberships, func(i, j int) bool {
		if observation.Memberships[i].Release != observation.Memberships[j].Release {
			return observation.Memberships[i].Release < observation.Memberships[j].Release
		}
		return observation.Memberships[i].ProwJobName < observation.Memberships[j].ProwJobName
	})
	sort.Slice(observation.Analyses, func(i, j int) bool {
		if observation.Analyses[i].Target.Release != observation.Analyses[j].Target.Release {
			return observation.Analyses[i].Target.Release < observation.Analyses[j].Target.Release
		}
		return observation.Analyses[i].Target.ProwJobName < observation.Analyses[j].Target.ProwJobName
	})
	sort.Slice(observation.Collection, func(i, j int) bool {
		a, b := observation.Collection[i], observation.Collection[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Release != b.Release {
			return a.Release < b.Release
		}
		return a.ProwJobName < b.ProwJobName
	})
	finished := time.Now().UTC()
	observation.ObservedAt = finished
	observation.Complete = status.Failed == 0
	status.FinishedAt = &finished
	status.HasData = true
	if observation.Complete {
		status.State = "ready"
	} else {
		status.State = "partial"
	}
	if err := observation.Validate(planDigest); err != nil {
		return nil, status, fmt.Errorf("validate Sippy observation: %w", err)
	}
	fmt.Fprintf(logWriter, "sippy: collection %s: %d/%d requests succeeded\n", status.State, status.Total-status.Failed, status.Total)
	return observation, status, nil
}
