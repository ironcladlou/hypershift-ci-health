package retests

import "time"

type RunStatus string

const (
	RunSuccess RunStatus = "success"
	RunFailure RunStatus = "failure"
	RunAborted RunStatus = "aborted"
	RunPending RunStatus = "pending"
)

type JobRun struct {
	ID     string    `json:"id"`
	Status RunStatus `json:"status"`
}

type PRJobResult struct {
	Name     string `json:"name"`
	Runs     int    `json:"runs"`
	Failures int    `json:"failures"`
	Aborts   int    `json:"aborts"`
	Retests  int    `json:"retests"`
}

type PRResult struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Author    string        `json:"author"`
	MergedAt  time.Time     `json:"merged_at"`
	HeadSHA   string        `json:"head_sha"`
	Jobs      []PRJobResult `json:"jobs"`
	MaxRetest int           `json:"max_retests"`
}

type JobSummary struct {
	Name          string  `json:"name"`
	DisplayName   string  `json:"display_name"`
	TotalRuns     int     `json:"total_runs"`
	TotalFailures int     `json:"total_failures"`
	PassRate      float64 `json:"pass_rate"`
	MedianRetests int     `json:"median_retests"`
	PRsAffected   int     `json:"prs_affected"`
}

type Summary struct {
	TotalRetestRounds        int          `json:"total_retest_rounds"`
	MedianRetests            int          `json:"median_retests_per_pr"`
	P90Retests               int          `json:"p90_retests_per_pr"`
	P95Retests               int          `json:"p95_retests_per_pr"`
	FirstTryMergeProbability float64      `json:"first_try_merge_probability"`
	PerJob                   []JobSummary `json:"per_job"`
	QueueBlockers            []string     `json:"queue_blockers"`
}

type AnalysisResult struct {
	GeneratedAt time.Time  `json:"generated_at"`
	WindowDays  int        `json:"window_days"`
	Org         string     `json:"org"`
	Repo        string     `json:"repo"`
	PRsAnalyzed int        `json:"prs_analyzed"`
	PRs         []PRResult `json:"prs"`
	Summary     Summary    `json:"summary"`
}

type BlockingJob struct {
	Name        string
	ProwJobName string
}

var BlockingJobs = []BlockingJob{
	{Name: "e2e-aws", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws"},
	{Name: "e2e-aks", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aks"},
	{Name: "e2e-azure-v2-self-managed", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-azure-v2-self-managed"},
	{Name: "e2e-aws-upgrade-hypershift-operator", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws-upgrade-hypershift-operator"},
	{Name: "e2e-v2-gke", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-v2-gke"},
	{Name: "e2e-aws-4-22", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aws-4-22"},
	{Name: "e2e-aks-4-22", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-aks-4-22"},
	{Name: "e2e-kubevirt-aws-ovn-reduced", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-kubevirt-aws-ovn-reduced"},
	{Name: "e2e-v2-aws", ProwJobName: "pull-ci-openshift-hypershift-main-e2e-v2-aws"},
}

type ProwPRHistory struct {
	Commits []CommitColumn
	Jobs    []ProwJobHistory
}

type CommitColumn struct {
	SHA     string
	Colspan int
}

type ProwJobHistory struct {
	Name string
	Runs []ProwJobRun
}

type ProwJobRun struct {
	ID        string
	Status    RunStatus
	CommitSHA string
}

type MergedPR struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	Author   string    `json:"author"`
	MergedAt time.Time `json:"merged_at"`
	HeadSHA  string    `json:"head_sha"`
}
