package sippy

import (
	"time"
)

// Sippy API response types

type SippyJob struct {
	Name                   string  `json:"name"`
	CurrentPassPercentage  float64 `json:"current_pass_percentage"`
	PreviousPassPercentage float64 `json:"previous_pass_percentage"`
	NetImprovement         float64 `json:"net_improvement"`
	CurrentRuns            int     `json:"current_runs"`
	CurrentFails           int     `json:"current_fails"`
	PreviousRuns           int     `json:"previous_runs"`
}

type SippyJobAnalysisPeriod struct {
	TotalRuns   int            `json:"total_runs"`
	ResultCount map[string]int `json:"result_count"`
}

type SippyJobAnalysisResponse struct {
	ByPeriod map[string]SippyJobAnalysisPeriod `json:"by_period"`
}

type SippyJobListItem struct {
	Name     string   `json:"name"`
	Variants []string `json:"variants"`
}

type SippyTestOutput struct {
	ProwJobName string `json:"prow_job_name"`
}

type SippyTestFailure struct {
	TestName string            `json:"test_name"`
	Outputs  []SippyTestOutput `json:"outputs"`
}

type SippyRecentFailuresResponse struct {
	Rows []SippyTestFailure `json:"rows"`
}

// Transformed output types served to the frontend

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

type HealthSnapshot struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Windows     map[string]*WindowData `json:"windows"`
	Platforms   []string               `json:"platforms"`
	Releases    []string               `json:"releases"`
}
