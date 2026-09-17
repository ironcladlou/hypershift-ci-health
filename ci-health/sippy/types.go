package sippy

type AnalysisPeriod struct {
	TotalRuns   int            `json:"total_runs"`
	ResultCount map[string]int `json:"result_count"`
}

type jobAnalysisResponse struct {
	ByPeriod map[string]AnalysisPeriod `json:"by_period"`
}

type jobListItem struct {
	Name     string   `json:"name"`
	Variants []string `json:"variants"`
}

type TestOutput struct {
	ProwJobName string `json:"prow_job_name"`
}
type RecentFailure struct {
	TestName string       `json:"test_name"`
	Outputs  []TestOutput `json:"outputs"`
}
type recentFailuresResponse struct {
	Rows []RecentFailure `json:"rows"`
}
