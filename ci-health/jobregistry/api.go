package jobregistry

// Registry is the stable, serializable registry contract.
type Registry struct {
	// APIVersion identifies the schema used to serialize the registry.
	APIVersion string `json:"api_version"`
	// Jobs contains every discovered job, ordered by stable job ID.
	Jobs []Job `json:"jobs"`
}

// Job describes one Prow job. A Prow job name is its globally unique, stable
// identity and is also retained as Name to keep identity and display concerns
// separate in the serialized contract.
type Job struct {
	// ID is the globally unique, stable identity of the Prow job.
	ID string `json:"id"`
	// Name is the current Prow job name displayed by reporting tools.
	Name string `json:"name"`
	// Repository is the owner/name repository associated with the Prow job.
	Repository string `json:"repository,omitempty"`
	// Context is the Prow context presented on pull requests. It is empty when
	// the generated job definition does not declare one.
	Context string `json:"context,omitempty"`
	// Type identifies the Prow job type and is either "periodic" or "presubmit".
	Type string `json:"type"`
	// Source canonically locates the generated job definition in openshift/release.
	Source Source `json:"source"`
	// Versions lists the concrete OpenShift releases tested by the job. It is
	// empty when no concrete release can be inferred; branch names such as main
	// are not versions.
	Versions []string `json:"versions"`
	// Platforms lists each infrastructure platform inferred from the job name,
	// context, and labels. It is empty when no platform can be inferred.
	Platforms []string `json:"platforms"`
	// E2EFramework identifies the HyperShift E2E framework as "v1" or "v2";
	// non-E2E jobs use "none".
	E2EFramework string `json:"e2e_framework"`
	// Variant is the ci-operator configuration variant from the generated Prow
	// job label. It is omitted when the job has no variant label.
	Variant string `json:"variant,omitempty"`
	// Presubmit contains presubmit-only behavior and is omitted for periodics.
	Presubmit *Presubmit `json:"presubmit,omitempty"`
	// ReleaseController lists every release-controller verification declaration
	// that refers to this job. An empty list means no declaration refers to it.
	ReleaseController []ReleaseControllerParticipation `json:"release_controller"`
	// SippyURL is a deterministic Sippy navigation URL. A non-nil URL does not
	// guarantee that Sippy currently has data for the job. It is nil when no
	// reliable Sippy route can be constructed.
	SippyURL *string `json:"sippy_url"`
	// ProwJobHistoryURL links to the job's build history in OpenShift Prow. The
	// URL does not guarantee that the job has any recorded runs.
	ProwJobHistoryURL string `json:"prow_job_history_url"`
}

// Presubmit contains properties that do not apply to periodic jobs.
type Presubmit struct {
	// Required reports whether the job is non-optional when it applies. It is
	// the inverse of Prow's optional field and does not imply AlwaysRun.
	Required bool `json:"required"`
	// AlwaysRun reports whether Prow automatically runs the job for every
	// matching pull request, subject to its other conditions.
	AlwaysRun bool `json:"always_run"`
	// Branches contains regular expressions selecting branches where the job
	// applies. An empty list means all branches not excluded by SkipBranches.
	Branches []string `json:"branches"`
	// SkipBranches contains regular expressions excluding branches from the job.
	SkipBranches []string `json:"skip_branches"`
	// RunIfChanged is a regular expression that conditionally runs the job when
	// a matching file changes. It is omitted when the job has no such condition.
	RunIfChanged string `json:"run_if_changed,omitempty"`
	// SkipIfOnlyChanged is a regular expression that skips the job when every
	// changed file matches. It is omitted when the job has no such condition.
	SkipIfOnlyChanged string `json:"skip_if_only_changed,omitempty"`
}

// ReleaseControllerParticipation describes how a job participates in one
// release stream's payload acceptance process.
type ReleaseControllerParticipation struct {
	// Stream identifies the release stream and provides navigation links to it.
	Stream ReleaseControllerStream `json:"stream"`
	// Verification describes the verify-map entry that refers to the job.
	Verification ReleaseControllerVerification `json:"verification"`
	// Source canonically locates the declaration in openshift/release.
	Source Source `json:"source"`
}

// ReleaseControllerStream identifies a stream derived from a release-controller
// configuration's name.
type ReleaseControllerStream struct {
	// Name is the exact release-controller stream name.
	Name string `json:"name"`
	// Release is the OpenShift major.minor release parsed from Name. It is empty
	// when Name does not use a recognized OCP CI or Nightly form.
	Release string `json:"release"`
	// Kind is "ci" or "nightly" when it can be parsed from Name; a multi-arch
	// nightly remains kind "nightly" and uses Architecture "multi".
	Kind string `json:"kind"`
	// Architecture is the stream architecture. OCP streams without an explicit
	// architecture suffix use "amd64". It is empty for unrecognized names.
	Architecture string `json:"architecture"`
	// EndOfLife is the stream's declared release-controller end-of-life state.
	EndOfLife bool `json:"end_of_life"`
	// SippyURL links to Sippy's stream overview. It is nil when the stream name
	// cannot be associated reliably with a Sippy release, architecture, and stream.
	SippyURL *string `json:"sippy_url"`
	// ReleaseStatusURL links to the release-controller payload status page. The
	// URL is a deterministic navigation hint and is not validated.
	ReleaseStatusURL string `json:"release_status_url"`
}

// ReleaseControllerVerification describes one entry in a release-controller
// configuration's verify map.
type ReleaseControllerVerification struct {
	// Name is the verify-map key, unique within its release stream.
	Name string `json:"name"`
	// Role is the effective participation mode: "blocking", "informing",
	// "async", or "disabled".
	Role string `json:"role"`
	// Optional reports whether failure is allowed without rejecting the payload.
	Optional bool `json:"optional"`
	// Disabled reports whether the declaration is prevented from participating.
	Disabled bool `json:"disabled"`
	// Async reports whether release acceptance proceeds without waiting for the
	// job. Release-controller defines this mode for optional verifications.
	Async bool `json:"async"`
	// Upgrade reports whether release-controller uses the verification to verify
	// upgrades. This does not classify the job itself as an upgrade test.
	Upgrade bool `json:"upgrade"`
	// UpgradeFrom overrides the default source used for upgrade verification. It
	// is omitted when release-controller applies its stream-specific default.
	UpgradeFrom string `json:"upgrade_from,omitempty"`
	// MaxRetries is the maximum number of retry attempts after the initial run.
	MaxRetries int `json:"max_retries"`
	// Aggregated describes aggregated release analysis and is omitted for a
	// normal single-run verification.
	Aggregated *ReleaseControllerAggregation `json:"aggregated,omitempty"`
	// MultiJobAnalysis reports whether the job analyzes results from multiple
	// other jobs associated with the payload.
	MultiJobAnalysis bool `json:"multi_job_analysis"`
}

// ReleaseControllerAggregation describes release-controller's repeated
// analysis and optional aggregation job.
type ReleaseControllerAggregation struct {
	// AnalysisJobCount is the number of asynchronous analysis jobs to execute.
	AnalysisJobCount int `json:"analysis_job_count"`
	// ProwJobName is the explicit aggregation Prow job. It is omitted when
	// release-controller uses its default aggregation job.
	ProwJobName string `json:"prow_job_name,omitempty"`
}

// Source is a canonical reference into openshift/release. The job ID selects
// the entry within the generated Prow configuration file.
type Source struct {
	// Repository is the canonical GitHub owner and repository name.
	Repository string `json:"repository"`
	// Path is the repository-relative path to the generated Prow configuration.
	Path string `json:"path"`
	// URL links to Path on the main branch of Repository.
	URL string `json:"url"`
	// Declaration identifies an entry within Path. It is omitted when the job ID
	// itself is sufficient to select the source definition.
	Declaration string `json:"declaration,omitempty"`
}
