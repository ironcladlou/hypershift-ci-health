package sippy

import "time"

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

type SippyJobRun struct {
	Timestamp     int64  `json:"timestamp"`
	Job           string `json:"job"`
	OverallResult string `json:"overall_result"`
	Succeeded     bool   `json:"succeeded"`
	TestFlakes    int    `json:"test_flakes"`
}

type SippyJobRunsResponse struct {
	Rows []SippyJobRun `json:"rows"`
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
	Name       string                    `json:"name"`
	Prow       string                    `json:"prow"`
	Release    string                    `json:"release"`
	Label      string                    `json:"label"`
	Rate       *float64                  `json:"rate"`
	Prev       *float64                  `json:"prev"`
	PrevRuns   int                       `json:"prev_runs"`
	Trend      *float64                  `json:"trend"`
	Runs       int                       `json:"runs"`
	Fails      int                       `json:"fails"`
	TestFails  int                       `json:"test_fails"`
	InfraFails int                       `json:"infra_fails"`
	SparkRuns  int                       `json:"spark_runs"`
	Sparkline  map[string]*SparklineSlot `json:"sparkline"`
	FlakyRuns  int                       `json:"flaky_runs"`
	TotalRuns  int                       `json:"total_runs"`
}

type JobHealth struct {
	Name        string                    `json:"name"`
	Prow        string                    `json:"prow"`
	Platform    string                    `json:"platform"`
	Version     string                    `json:"version"`
	Rate        float64                   `json:"rate"`
	Prev        float64                   `json:"prev"`
	PrevRuns    int                       `json:"prev_runs"`
	Trend       *float64                  `json:"trend"`
	Runs        int                       `json:"runs"`
	Fails       int                       `json:"fails"`
	TestFails   int                       `json:"test_fails"`
	InfraFails  int                       `json:"infra_fails"`
	SparkRuns   int                       `json:"spark_runs"`
	Periodics   []PeriodicJobHealth       `json:"periodics"`
	Sparkline   map[string]*SparklineSlot `json:"sparkline"`
	Correlation *Correlation              `json:"correlation"`
}

type Alert struct {
	TestName     string   `json:"test_name"`
	FailureCount int      `json:"failure_count"`
	Jobs         []string `json:"jobs"`
}

type WindowData struct {
	Jobs   []JobHealth `json:"jobs"`
	Alerts []Alert     `json:"alerts"`
}

type HealthSnapshot struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Windows     map[string]*WindowData `json:"windows"`
	Platforms   []string               `json:"platforms"`
	Versions    []string               `json:"versions"`
}
