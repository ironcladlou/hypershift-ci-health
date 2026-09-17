package jobregistry

// Registry is the stable, serializable registry contract.
type Registry struct {
	// APIVersion identifies the schema used to serialize the registry.
	APIVersion string `json:"api_version" doc:"Schema version used to serialize this registry" example:"job-registry/v7" enum:"job-registry/v7"`
	// Jobs contains every discovered job, ordered by stable job ID.
	Jobs []Job `json:"jobs" doc:"Discovered jobs ordered by stable job ID"`
}

// SippyIngestion records whether Sippy is intended to ingest analysis data for
// a presubmit. It describes configured availability, not whether the job has
// produced runs.
type SippyIngestion struct {
	Enabled bool   `json:"enabled" doc:"Whether Sippy is configured to ingest analysis for the presubmit"`
	Basis   string `json:"basis" doc:"Reason for the configured ingestion state"`
}

// JobType identifies the Prow execution mode for a job.
type JobType string

const (
	JobTypePeriodic  JobType = "periodic"
	JobTypePresubmit JobType = "presubmit"
)

// E2EFramework identifies the HyperShift end-to-end test framework generation.
type E2EFramework string

const (
	E2EFrameworkNone E2EFramework = "none"
	E2EFrameworkV1   E2EFramework = "v1"
	E2EFrameworkV2   E2EFramework = "v2"
)

// PeriodicCounterpartSource identifies who asserted a periodic relationship.
type PeriodicCounterpartSource string

const (
	PeriodicCounterpartSourceRegistryManual   PeriodicCounterpartSource = "registry-manual"
	PeriodicCounterpartSourceRegistryProposal PeriodicCounterpartSource = "registry-proposal"
)

// PeriodicCounterpartVerification identifies the review state of a periodic
// relationship.
type PeriodicCounterpartVerification string

const (
	PeriodicCounterpartVerificationHuman       PeriodicCounterpartVerification = "human-verified"
	PeriodicCounterpartVerificationNeedsReview PeriodicCounterpartVerification = "needs-review"
)

// PeriodicCounterpart identifies a periodic believed to exercise the same
// scenario as its containing presubmit. Prow does not currently express this
// relationship, so the registry includes its source, verification, and rationale.
type PeriodicCounterpart struct {
	JobID         string                          `json:"job_id" doc:"Stable ID of the associated periodic job"`
	TestedRelease string                          `json:"tested_release" doc:"Concrete OpenShift release tested by the periodic" example:"4.22"`
	Source        PeriodicCounterpartSource       `json:"source" doc:"Who asserted this relationship" enum:"registry-manual,registry-proposal"`
	Verification  PeriodicCounterpartVerification `json:"verification" doc:"Review state of this relationship" enum:"human-verified,needs-review"`
	Rationale     string                          `json:"rationale" doc:"Evidence supporting this relationship"`
}

// Job describes one Prow job. A Prow job name is its globally unique, stable
// identity and is also retained as Name to keep identity and display concerns
// separate in the serialized contract.
type Job struct {
	// ID is the globally unique, stable identity of the Prow job.
	ID string `json:"id" doc:"Globally unique, stable identity of the Prow job"`
	// Name is the current Prow job name displayed by reporting tools.
	Name string `json:"name" doc:"Current Prow job name used for display"`
	// Repository is the owner/name repository associated with the Prow job.
	Repository string `json:"repository,omitempty" doc:"Owner/name repository associated with the Prow job" example:"openshift/hypershift"`
	// Context is the Prow context presented on pull requests. It is empty when
	// the generated job definition does not declare one.
	Context string `json:"context,omitempty" doc:"Prow context presented on pull requests"`
	// Type identifies the Prow job type and is either "periodic" or "presubmit".
	Type JobType `json:"type" doc:"Prow job type" enum:"periodic,presubmit"`
	// Source canonically locates the generated job definition in openshift/release.
	Source Source `json:"source" doc:"Canonical source of the generated job definition"`
	// Versions lists the concrete OpenShift releases tested by the job. It is
	// empty when no concrete release can be inferred; branch names such as main
	// are not versions.
	Versions []string `json:"versions" doc:"Concrete OpenShift releases tested by the job"`
	// Platforms lists each infrastructure platform inferred from the job name,
	// context, and labels. It is empty when no platform can be inferred.
	Platforms []string `json:"platforms" doc:"Infrastructure platforms inferred for the job"`
	// E2EFramework identifies the HyperShift E2E framework as "v1" or "v2";
	// non-E2E jobs use "none".
	E2EFramework E2EFramework `json:"e2e_framework" doc:"HyperShift end-to-end framework used by the job" enum:"none,v1,v2"`
	// Variant is the ci-operator configuration variant from the generated Prow
	// job label. It is omitted when the job has no variant label.
	Variant string `json:"variant,omitempty" doc:"ci-operator configuration variant"`
	// Presubmit contains presubmit-only behavior and is omitted for periodics.
	Presubmit *Presubmit `json:"presubmit,omitempty" doc:"Presubmit-only behavior; absent for periodic jobs"`
	// ReleaseController lists every release-controller verification declaration
	// that refers to this job. An empty list means no declaration refers to it.
	ReleaseController []ReleaseControllerParticipation `json:"release_controller" doc:"Release-controller verification declarations referring to this job"`
	// SippyURL is a deterministic Sippy navigation URL. A non-nil URL does not
	// guarantee that Sippy currently has data for the job. It is nil when no
	// reliable Sippy route can be constructed.
	SippyURL *string `json:"sippy_url" doc:"Sippy navigation URL when a reliable route can be constructed" format:"uri"`
	// ProwJobHistoryURL links to the job's build history in OpenShift Prow. The
	// URL does not guarantee that the job has any recorded runs.
	ProwJobHistoryURL string `json:"prow_job_history_url" doc:"OpenShift Prow build-history URL" format:"uri"`
}

// Presubmit contains properties that do not apply to periodic jobs.
type Presubmit struct {
	// TargetBranch is the Prow branch receiving the pull request.
	TargetBranch string `json:"target_branch" doc:"Prow branch receiving the pull request" example:"main"`
	// TargetRelease is the dashboard release line represented by TargetBranch.
	// It can differ from a counterpart's TestedRelease for compatibility jobs.
	TargetRelease string `json:"target_release" doc:"Dashboard release line represented by the target branch" example:"5.0"`
	// SippyIngestion records whether the collector should request analysis for
	// this presubmit and why.
	SippyIngestion SippyIngestion `json:"sippy_ingestion" doc:"Whether Sippy should request analysis and why"`
	// Required reports whether the job is non-optional when it applies. It is
	// the inverse of Prow's optional field and does not imply AlwaysRun.
	Required bool `json:"required" doc:"Whether the job is non-optional when it applies"`
	// AlwaysRun reports whether Prow automatically runs the job for every
	// matching pull request, subject to its other conditions.
	AlwaysRun bool `json:"always_run" doc:"Whether Prow automatically runs the job for every matching pull request"`
	// Branches contains regular expressions selecting branches where the job
	// applies. An empty list means all branches not excluded by SkipBranches.
	Branches []string `json:"branches" doc:"Regular expressions selecting branches where the job applies"`
	// SkipBranches contains regular expressions excluding branches from the job.
	SkipBranches []string `json:"skip_branches" doc:"Regular expressions excluding branches from the job"`
	// RunIfChanged is a regular expression that conditionally runs the job when
	// a matching file changes. It is omitted when the job has no such condition.
	RunIfChanged string `json:"run_if_changed,omitempty" doc:"Regular expression selecting file changes that cause the job to run"`
	// SkipIfOnlyChanged is a regular expression that skips the job when every
	// changed file matches. It is omitted when the job has no such condition.
	SkipIfOnlyChanged string `json:"skip_if_only_changed,omitempty" doc:"Regular expression that skips the job when every changed file matches"`
	// PeriodicCounterparts contains registry-owned periodic associations and
	// review proposals. Verification distinguishes reviewed mappings from
	// candidates. An empty list means no counterpart has been identified.
	PeriodicCounterparts []PeriodicCounterpart `json:"periodic_counterparts" doc:"Registry-owned associations with periodic jobs"`
}

// ReleaseControllerParticipation describes how a job participates in one
// release stream's payload acceptance process.
type ReleaseControllerParticipation struct {
	// Stream identifies the release stream and provides navigation links to it.
	Stream ReleaseControllerStream `json:"stream" doc:"Release stream in which the job participates"`
	// Verification describes the verify-map entry that refers to the job.
	Verification ReleaseControllerVerification `json:"verification" doc:"Verify-map entry referring to the job"`
	// Source canonically locates the declaration in openshift/release.
	Source Source `json:"source" doc:"Canonical source of the release-controller declaration"`
}

// ReleaseControllerStreamKind identifies a recognized payload stream family.
type ReleaseControllerStreamKind string

const (
	ReleaseControllerStreamKindCI      ReleaseControllerStreamKind = "ci"
	ReleaseControllerStreamKindNightly ReleaseControllerStreamKind = "nightly"
)

// ReleaseControllerStream identifies a stream derived from a release-controller
// configuration's name.
type ReleaseControllerStream struct {
	// Name is the exact release-controller stream name.
	Name string `json:"name" doc:"Exact release-controller stream name"`
	// Release is the OpenShift major.minor release parsed from Name. It is empty
	// when Name does not use a recognized OCP CI or Nightly form.
	Release string `json:"release" doc:"OpenShift major.minor release parsed from the stream name" example:"4.22"`
	// Kind is "ci" or "nightly" when it can be parsed from Name; a multi-arch
	// nightly remains kind "nightly" and uses Architecture "multi".
	Kind ReleaseControllerStreamKind `json:"kind" doc:"Parsed stream kind (ci or nightly), or empty for an unrecognized name"`
	// Architecture is the stream architecture. OCP streams without an explicit
	// architecture suffix use "amd64". It is empty for unrecognized names.
	Architecture string `json:"architecture" doc:"Stream architecture, or empty for an unrecognized name" example:"amd64"`
	// EndOfLife is the stream's declared release-controller end-of-life state.
	EndOfLife bool `json:"end_of_life" doc:"Declared release-controller end-of-life state"`
	// SippyURL links to Sippy's stream overview. It is nil when the stream name
	// cannot be associated reliably with a Sippy release, architecture, and stream.
	SippyURL *string `json:"sippy_url" doc:"Sippy stream-overview URL when the stream can be parsed" format:"uri"`
	// ReleaseStatusURL links to the release-controller payload status page. The
	// URL is a deterministic navigation hint and is not validated.
	ReleaseStatusURL string `json:"release_status_url" doc:"Release-controller payload-status URL" format:"uri"`
}

// ReleaseControllerRole identifies how a verification affects payload acceptance.
type ReleaseControllerRole string

const (
	ReleaseControllerRoleBlocking  ReleaseControllerRole = "blocking"
	ReleaseControllerRoleInforming ReleaseControllerRole = "informing"
	ReleaseControllerRoleAsync     ReleaseControllerRole = "async"
	ReleaseControllerRoleDisabled  ReleaseControllerRole = "disabled"
)

// ReleaseControllerVerification describes one entry in a release-controller
// configuration's verify map.
type ReleaseControllerVerification struct {
	// Name is the verify-map key, unique within its release stream.
	Name string `json:"name" doc:"Verify-map key, unique within its release stream"`
	// Role is the effective participation mode: "blocking", "informing",
	// "async", or "disabled".
	Role ReleaseControllerRole `json:"role" doc:"Effective participation mode" enum:"blocking,informing,async,disabled"`
	// Optional reports whether failure is allowed without rejecting the payload.
	Optional bool `json:"optional" doc:"Whether failure is allowed without rejecting the payload"`
	// Disabled reports whether the declaration is prevented from participating.
	Disabled bool `json:"disabled" doc:"Whether the declaration is prevented from participating"`
	// Async reports whether release acceptance proceeds without waiting for the
	// job. Release-controller defines this mode for optional verifications.
	Async bool `json:"async" doc:"Whether release acceptance proceeds without waiting for the job"`
	// Upgrade reports whether release-controller uses the verification to verify
	// upgrades. This does not classify the job itself as an upgrade test.
	Upgrade bool `json:"upgrade" doc:"Whether release-controller uses this verification to verify upgrades"`
	// UpgradeFrom overrides the default source used for upgrade verification. It
	// is omitted when release-controller applies its stream-specific default.
	UpgradeFrom string `json:"upgrade_from,omitempty" doc:"Explicit source for upgrade verification"`
	// MaxRetries is the maximum number of retry attempts after the initial run.
	MaxRetries int `json:"max_retries" doc:"Maximum retry attempts after the initial run" minimum:"0"`
	// Aggregated describes aggregated release analysis and is omitted for a
	// normal single-run verification.
	Aggregated *ReleaseControllerAggregation `json:"aggregated,omitempty" doc:"Repeated release analysis for an aggregated verification"`
	// MultiJobAnalysis reports whether the job analyzes results from multiple
	// other jobs associated with the payload.
	MultiJobAnalysis bool `json:"multi_job_analysis" doc:"Whether this job analyzes results from multiple payload jobs"`
}

// ReleaseControllerAggregation describes release-controller's repeated
// analysis and optional aggregation job.
type ReleaseControllerAggregation struct {
	// AnalysisJobCount is the number of asynchronous analysis jobs to execute.
	AnalysisJobCount int `json:"analysis_job_count" doc:"Number of asynchronous analysis jobs to execute" minimum:"0"`
	// ProwJobName is the explicit aggregation Prow job. It is omitted when
	// release-controller uses its default aggregation job.
	ProwJobName string `json:"prow_job_name,omitempty" doc:"Explicit aggregation Prow job, if configured"`
}

// Source is a canonical reference into openshift/release. The job ID selects
// the entry within the generated Prow configuration file.
type Source struct {
	// Repository is the canonical GitHub owner and repository name.
	Repository string `json:"repository" doc:"Canonical GitHub owner and repository name" example:"openshift/release"`
	// Path is the repository-relative path to the generated Prow configuration.
	Path string `json:"path" doc:"Repository-relative path to the source file"`
	// URL links to Path on the main branch of Repository.
	URL string `json:"url" doc:"Link to the source file on the repository's main branch" format:"uri"`
	// Declaration identifies an entry within Path. It is omitted when the job ID
	// itself is sufficient to select the source definition.
	Declaration string `json:"declaration,omitempty" doc:"Entry within the source file when the job ID is not sufficient"`
}
