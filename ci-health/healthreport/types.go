package healthreport

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
)

const CurrentAPIVersion = "health-report/v1"

type SparklineSlot struct {
	TotalRuns   int            `json:"total_runs"`
	ResultCount map[string]int `json:"result_count"`
}

type Correlation struct {
	Correlated int   `json:"correlated"`
	PreFails   int   `json:"pre_fails"`
	Indices    []int `json:"indices"`
}

type PeriodicJobHealth struct {
	ID                       string                    `json:"id"`
	Name                     string                    `json:"name"`
	Prow                     string                    `json:"prow"`
	Release                  string                    `json:"release"`
	Label                    string                    `json:"label"`
	RelationshipSource       string                    `json:"relationship_source,omitempty"`
	RelationshipVerification string                    `json:"relationship_verification,omitempty"`
	RelationshipRationale    string                    `json:"relationship_rationale,omitempty"`
	Rate                     *float64                  `json:"rate"`
	Prev                     *float64                  `json:"prev"`
	PrevRuns                 int                       `json:"prev_runs"`
	Trend                    *float64                  `json:"trend"`
	Runs                     int                       `json:"runs"`
	Fails                    int                       `json:"fails"`
	TestFails                int                       `json:"test_fails"`
	InfraFails               int                       `json:"infra_fails"`
	SparkRuns                int                       `json:"spark_runs"`
	Sparkline                map[string]*SparklineSlot `json:"sparkline"`
}

type ReleasePayloadParticipation struct {
	StreamName       string  `json:"stream_name"`
	StreamKind       string  `json:"stream_kind"`
	Architecture     string  `json:"architecture"`
	VerificationName string  `json:"verification_name"`
	StreamSippyURL   *string `json:"stream_sippy_url"`
	ReleaseStatusURL string  `json:"release_status_url"`
}

type PayloadBlockingJobHealth struct {
	PeriodicJobHealth
	Platforms      []string                      `json:"platforms"`
	Participations []ReleasePayloadParticipation `json:"participations"`
}

type ComponentReadinessJobHealth struct {
	PeriodicJobHealth
	Platforms       []string `json:"platforms"`
	RegistryMissing bool     `json:"registry_missing,omitempty"`
}

type JobHealth struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Prow          string                    `json:"prow"`
	TargetBranch  string                    `json:"target_branch"`
	TargetRelease string                    `json:"target_release"`
	Platforms     []string                  `json:"platforms"`
	Role          string                    `json:"role"`
	RoleLabel     string                    `json:"role_label"`
	Rate          float64                   `json:"rate"`
	Prev          float64                   `json:"prev"`
	PrevRuns      int                       `json:"prev_runs"`
	Trend         *float64                  `json:"trend"`
	Runs          int                       `json:"runs"`
	Fails         int                       `json:"fails"`
	TestFails     int                       `json:"test_fails"`
	InfraFails    int                       `json:"infra_fails"`
	SparkRuns     int                       `json:"spark_runs"`
	Periodics     []PeriodicJobHealth       `json:"periodics"`
	Sparkline     map[string]*SparklineSlot `json:"sparkline"`
	Correlation   *Correlation              `json:"correlation"`
}

type Alert struct {
	TestName     string   `json:"test_name"`
	FailureCount int      `json:"failure_count"`
	Jobs         []string `json:"jobs"`
}

type WindowData struct {
	Jobs                   []JobHealth                   `json:"jobs"`
	PayloadBlockingJobs    []PayloadBlockingJobHealth    `json:"payload_blocking_jobs"`
	ComponentReadinessJobs []ComponentReadinessJobHealth `json:"component_readiness_jobs"`
	Alerts                 []Alert                       `json:"alerts"`
}

type CollectionResult struct {
	Kind        string `json:"kind"`
	Release     string `json:"release,omitempty"`
	ProwJobName string `json:"prow_job_name,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Report struct {
	APIVersion           string                 `json:"api_version"`
	RegistryDigest       string                 `json:"registry_digest"`
	PlanDigest           string                 `json:"plan_digest"`
	ObservationDigest    string                 `json:"observation_digest"`
	GeneratedAt          time.Time              `json:"generated_at"`
	ObservationStartedAt time.Time              `json:"observation_started_at"`
	Complete             bool                   `json:"complete"`
	Collection           []CollectionResult     `json:"collection"`
	Windows              map[string]*WindowData `json:"windows"`
	Platforms            []string               `json:"platforms"`
	Releases             []string               `json:"releases"`
}

func (r *Report) Validate(registryDigest, planDigest, observationDigest string) error {
	if r.APIVersion != CurrentAPIVersion {
		return fmt.Errorf("unsupported health report API version %q", r.APIVersion)
	}
	if r.RegistryDigest != registryDigest {
		return fmt.Errorf("health report registry digest %q does not match %q", r.RegistryDigest, registryDigest)
	}
	if r.PlanDigest != planDigest {
		return fmt.Errorf("health report plan digest %q does not match %q", r.PlanDigest, planDigest)
	}
	if r.ObservationDigest != observationDigest {
		return fmt.Errorf("health report observation digest %q does not match %q", r.ObservationDigest, observationDigest)
	}
	if r.GeneratedAt.Before(r.ObservationStartedAt) {
		return fmt.Errorf("health report predates its observation")
	}
	hasFailure := false
	for _, result := range r.Collection {
		if result.Error != "" {
			hasFailure = true
			break
		}
	}
	if r.Complete == hasFailure {
		return fmt.Errorf("health report completeness disagrees with collection results")
	}
	return nil
}

func LoadFile(path string, registry *jobregistry.Registry, plan *reportplan.Plan, observation *sippy.Observation) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read health report %q: %w", path, err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("load health report %q: %w", path, err)
	}
	planDigest, err := reportplan.Digest(plan)
	if err != nil {
		return nil, err
	}
	observationDigest, err := sippy.ObservationDigest(observation)
	if err != nil {
		return nil, err
	}
	if err := report.Validate(plan.RegistryDigest, planDigest, observationDigest); err != nil {
		return nil, fmt.Errorf("validate health report %q: %w", path, err)
	}
	expected, err := Evaluate(registry, plan, observation)
	if err != nil {
		return nil, fmt.Errorf("rebuild health report %q: %w", path, err)
	}
	if !reflect.DeepEqual(&report, expected) {
		return nil, fmt.Errorf("validate health report %q: artifact is not the canonical evaluation of its inputs", path)
	}
	return &report, nil
}
