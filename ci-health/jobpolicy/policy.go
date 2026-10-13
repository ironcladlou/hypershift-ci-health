// Package jobpolicy evaluates team assumptions over registry facts. It performs
// no collection and does not participate in report planning or health analysis.
package jobpolicy

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/releaseconfig"
	"go.yaml.in/yaml/v3"
)

const PolicyAPIVersion = "team-job-policy/v1"
const CatalogAPIVersion = "team-job-catalog/v1"
const AlgorithmVersion = "job-importance/v1"

//go:embed policies/hypershift.yaml
var defaultPolicy []byte

// Policy is reviewed team configuration. Group order takes precedence over
// dimensions, which are compared in their declared order. Job ID breaks ties.
type Policy struct {
	DevelopmentBranch  string                                  `json:"development_branch" yaml:"development_branch"`
	DevelopmentRelease string                                  `json:"development_release" yaml:"development_release" doc:"Explicit development release defining the ranking horizon"`
	APIVersion         string                                  `json:"api_version" yaml:"api_version"`
	Team               string                                  `json:"team" yaml:"team"`
	Version            string                                  `json:"version" yaml:"version"`
	Groups             []Group                                 `json:"groups" yaml:"groups"`
	Dimensions         []string                                `json:"dimensions" yaml:"dimensions" doc:"Ordered ranking dimensions: development-presubmit, non-e2e-presubmit, required-presubmit-periodic, release, and cloud"`
	CloudOrder         []string                                `json:"cloud_order" yaml:"cloud_order"`
	CloudAliases       map[string]string                       `json:"cloud_aliases" yaml:"cloud_aliases"`
	HealthObjectives   map[jobregistry.JobType]HealthObjective `json:"health_objectives" yaml:"health_objectives" doc:"Default assessment contracts by job type"`
	Cohorts            map[string]HealthCohort                 `json:"cohorts,omitempty" yaml:"cohorts" doc:"Named team-scoped selectors used by cross-job assessments"`
	Labels             []string                                `json:"labels,omitempty" yaml:"labels"`
	Annotations        map[string]string                       `json:"annotations,omitempty" yaml:"annotations"`
	Overrides          map[string]Override                     `json:"overrides,omitempty" yaml:"overrides"`
}

type Group struct {
	ID               string                                    `json:"id" yaml:"id"`
	Description      string                                    `json:"description" yaml:"description"`
	Match            string                                    `json:"match" yaml:"match" enum:"release-controller,required-auto-run,all"`
	Streams          []string                                  `json:"streams,omitempty" yaml:"streams"`
	StreamKinds      []jobregistry.ReleaseControllerStreamKind `json:"stream_kinds,omitempty" yaml:"stream_kinds" doc:"Stream families (ci or nightly); combined with streams when both are specified"`
	Roles            []jobregistry.ReleaseControllerRole       `json:"roles,omitempty" yaml:"roles"`
	HealthObjectives map[jobregistry.JobType]HealthObjective   `json:"health_objectives,omitempty" yaml:"health_objectives" doc:"Assessment replacements for matching jobs, by job type"`
}

type Override struct {
	Rationale       string            `json:"rationale" yaml:"rationale"`
	Group           string            `json:"group,omitempty" yaml:"group"`
	HealthObjective *HealthObjective  `json:"health_objective,omitempty" yaml:"health_objective"`
	Labels          []string          `json:"labels,omitempty" yaml:"labels"`
	Annotations     map[string]string `json:"annotations,omitempty" yaml:"annotations"`
}

type Importance struct {
	ReleaseScope         string   `json:"release_scope" enum:"in-horizon,upcoming,unknown" doc:"Upcoming jobs rank after every job within or without a known release horizon"`
	ScopeRelease         string   `json:"scope_release" doc:"Highest concrete tested or target release used to classify the job against the development release"`
	PresubmitBranchScope string   `json:"presubmit_branch_scope,omitempty" enum:"development,release,prospective,unknown" doc:"Target branch classification; prospective release-branch presubmits rank after active jobs"`
	Rank                 int      `json:"rank" minimum:"1" doc:"One-based global position within this catalog snapshot"`
	Group                string   `json:"group"`
	ResolvedRelease      string   `json:"resolved_release" doc:"Presubmit target release (development branch resolves to development release), otherwise highest concrete tested release; empty when unknown"`
	ResolvedCloud        string   `json:"resolved_cloud" doc:"Highest-priority recognized cloud, or empty when unknown"`
	MatchedRules         []string `json:"matched_rules"`
	RequiredPresubmitIDs []string `json:"required_presubmit_ids,omitempty" doc:"Required presubmits with non-prospective target branches whose verified registry counterparts include this periodic"`
}

type Metadata struct {
	Importance        Importance        `json:"importance"`
	HealthObjective   HealthObjective   `json:"health_objective"`
	Labels            []string          `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	OverrideRationale string            `json:"override_rationale,omitempty"`
}

type Entry struct {
	JobID        string   `json:"job_id"`
	TeamMetadata Metadata `json:"team_metadata"`
}

// Catalog contains exactly one reference per registry job in importance order.
// Its serialized policy and digests make canonical output reproducible.
type Catalog struct {
	APIVersion       string  `json:"api_version"`
	AlgorithmVersion string  `json:"algorithm_version"`
	RegistryDigest   string  `json:"registry_digest"`
	PolicyDigest     string  `json:"policy_digest"`
	Policy           Policy  `json:"policy"`
	Jobs             []Entry `json:"jobs"`
}

func Digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func DefaultPolicy() (Policy, error) { return ParsePolicy(defaultPolicy) }

func LoadPolicyFile(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicy(data)
}

func ParsePolicy(data []byte) (Policy, error) {
	var policy Policy
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&policy); err != nil {
		return policy, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return policy, fmt.Errorf("policy must contain exactly one YAML document")
	}
	if policy.DevelopmentBranch == "" {
		policy.DevelopmentBranch = releaseconfig.DevelopmentBranch
	}
	if policy.DevelopmentRelease == "" {
		policy.DevelopmentRelease = releaseconfig.DevelopmentRelease
	}
	return policy, policy.Validate()
}

var identifierRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func unique(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (p Policy) Validate() error {
	if err := releaseconfig.Validate(p.DevelopmentBranch, p.DevelopmentRelease); err != nil {
		return err
	}
	if p.APIVersion != PolicyAPIVersion || !identifierRE.MatchString(p.Team) || p.Version == "" {
		return fmt.Errorf("unsupported policy schema or missing team/version")
	}
	for id, cohort := range p.Cohorts {
		if !identifierRE.MatchString(id) {
			return fmt.Errorf("invalid health cohort identifier %q", id)
		}
		if err := cohort.validate(); err != nil {
			return fmt.Errorf("cohort %q: %w", id, err)
		}
	}
	for _, kind := range []jobregistry.JobType{jobregistry.JobTypePresubmit, jobregistry.JobTypePeriodic} {
		if _, found := p.HealthObjectives[kind]; !found {
			return fmt.Errorf("a default health objective is required for %s jobs", kind)
		}
	}
	if err := validateObjectives(p.HealthObjectives, p.Cohorts); err != nil {
		return err
	}
	if len(p.Dimensions) == 0 || !unique(p.Dimensions) {
		return fmt.Errorf("ranking dimensions must be nonempty and unique")
	}
	for _, dimension := range p.Dimensions {
		if dimension != "release" && dimension != "cloud" && dimension != "development-presubmit" && dimension != "non-e2e-presubmit" && dimension != "required-presubmit-periodic" {
			return fmt.Errorf("unknown ranking dimension %q", dimension)
		}
	}
	if len(p.CloudOrder) == 0 || !unique(p.CloudOrder) {
		return fmt.Errorf("cloud order must be nonempty and unique")
	}
	for alias, cloud := range p.CloudAliases {
		if alias == "" || slices.Contains(p.CloudOrder, alias) || !slices.Contains(p.CloudOrder, cloud) {
			return fmt.Errorf("invalid cloud alias %q -> %q", alias, cloud)
		}
	}
	groups := map[string]bool{}
	for i, group := range p.Groups {
		if !identifierRE.MatchString(group.ID) || groups[group.ID] || group.Description == "" {
			return fmt.Errorf("invalid or duplicate group %q", group.ID)
		}
		groups[group.ID] = true
		switch group.Match {
		case "release-controller":
			if (len(group.Streams) == 0 && len(group.StreamKinds) == 0) || len(group.Roles) == 0 || !unique(group.Streams) {
				return fmt.Errorf("release-controller group requires streams or stream kinds, and roles")
			}
			kinds := []string{}
			for _, kind := range group.StreamKinds {
				if kind != jobregistry.ReleaseControllerStreamKindCI && kind != jobregistry.ReleaseControllerStreamKindNightly {
					return fmt.Errorf("invalid stream kind %q", kind)
				}
				kinds = append(kinds, string(kind))
			}
			if !unique(kinds) {
				return fmt.Errorf("duplicate release-controller stream kinds")
			}
			roles := []string{}
			for _, role := range group.Roles {
				if role != jobregistry.ReleaseControllerRoleBlocking && role != jobregistry.ReleaseControllerRoleInforming && role != jobregistry.ReleaseControllerRoleAsync {
					return fmt.Errorf("invalid active role %q", role)
				}
				roles = append(roles, string(role))
			}
			if !unique(roles) {
				return fmt.Errorf("duplicate release-controller roles")
			}
		case "required-auto-run", "all":
			if len(group.Streams) > 0 || len(group.StreamKinds) > 0 || len(group.Roles) > 0 {
				return fmt.Errorf("streams, stream kinds, and roles only apply to release-controller groups")
			}
		default:
			return fmt.Errorf("unknown group match %q", group.Match)
		}
		if group.Match == "all" && i != len(p.Groups)-1 {
			return fmt.Errorf("catch-all group must be last")
		}
		if err := validateObjectives(group.HealthObjectives, p.Cohorts); err != nil {
			return fmt.Errorf("group %q: %w", group.ID, err)
		}
	}
	if len(p.Groups) == 0 || p.Groups[len(p.Groups)-1].Match != "all" {
		return fmt.Errorf("a final catch-all group is required")
	}
	for id, override := range p.Overrides {
		if id == "" || strings.TrimSpace(override.Rationale) == "" {
			return fmt.Errorf("override %q requires a rationale", id)
		}
		if override.Group != "" && !groups[override.Group] {
			return fmt.Errorf("override %q uses unknown group %q", id, override.Group)
		}
		if override.HealthObjective != nil {
			if err := override.HealthObjective.validate(p.Cohorts); err != nil {
				return err
			}
		}
	}
	return nil
}

func matches(job jobregistry.Job, group Group) bool {
	switch group.Match {
	case "all":
		return true
	case "required-auto-run":
		return job.Presubmit != nil && job.Presubmit.Required && job.Presubmit.AlwaysRun
	case "release-controller":
		for _, participation := range job.ReleaseController {
			streamMatches := (len(group.Streams) == 0 || slices.Contains(group.Streams, participation.Stream.Name)) && (len(group.StreamKinds) == 0 || slices.Contains(group.StreamKinds, participation.Stream.Kind))
			if !participation.Stream.EndOfLife && !participation.Verification.Disabled && participation.Verification.Role != jobregistry.ReleaseControllerRoleDisabled && streamMatches && slices.Contains(group.Roles, participation.Verification.Role) {
				return true
			}
		}
	}
	return false
}

func compareRelease(a, b string) int {
	amajor, aminor, aknown := releaseconfig.ParseRelease(a)
	bmajor, bminor, bknown := releaseconfig.ParseRelease(b)
	if aknown != bknown {
		if aknown {
			return -1
		}
		return 1
	}
	if amajor != bmajor {
		if amajor > bmajor {
			return -1
		}
		return 1
	}
	if aminor != bminor {
		if aminor > bminor {
			return -1
		}
		return 1
	}
	return 0
}

func presubmitBranchScope(job jobregistry.Job, policy Policy) string {
	if job.Presubmit == nil {
		return ""
	}
	presubmit := job.Presubmit
	if presubmit.TargetBranch == "" {
		return "unknown"
	}
	if presubmit.TargetBranch == policy.DevelopmentBranch {
		return "development"
	}
	if _, _, known := releaseconfig.ParseRelease(presubmit.TargetRelease); !known {
		return "unknown"
	}
	if compareRelease(presubmit.TargetRelease, policy.DevelopmentRelease) <= 0 {
		return "prospective"
	}
	return "release"
}

func Evaluate(registry *jobregistry.Registry, policy Policy) (*Catalog, error) {
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if err := registry.ValidateDevelopmentRelease(policy.DevelopmentBranch, policy.DevelopmentRelease); err != nil {
		return nil, err
	}
	index := registry.Index()
	requiredPresubmits := map[string][]string{}
	for _, job := range registry.Jobs {
		if job.Presubmit == nil || !job.Presubmit.Required || presubmitBranchScope(job, policy) == "prospective" {
			continue
		}
		for _, counterpart := range job.Presubmit.PeriodicCounterparts {
			requiredPresubmits[counterpart.JobID] = append(requiredPresubmits[counterpart.JobID], job.ID)
		}
	}
	for _, ids := range requiredPresubmits {
		sort.Strings(ids)
	}
	for id := range policy.Overrides {
		if index[id] == nil {
			return nil, fmt.Errorf("override references unknown job %q", id)
		}
		if objective := policy.Overrides[id].HealthObjective; objective != nil && objective.Assessment == AssessmentPresubmitSystemicHealth && index[id].Type != jobregistry.JobTypePresubmit {
			return nil, fmt.Errorf("presubmit systemic-health override cannot apply to periodic %q", id)
		}
	}
	registryDigest, err := Digest(registry)
	if err != nil {
		return nil, err
	}
	policyDigest, err := Digest(policy)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{APIVersion: CatalogAPIVersion, AlgorithmVersion: AlgorithmVersion, RegistryDigest: registryDigest, PolicyDigest: policyDigest, Policy: policy, Jobs: []Entry{}}
	groupOrder := map[string]int{}
	for i, group := range policy.Groups {
		groupOrder[group.ID] = i
	}
	cloudRank := func(cloud string) int {
		if i := slices.Index(policy.CloudOrder, cloud); i >= 0 {
			return i
		}
		return len(policy.CloudOrder)
	}
	for _, job := range registry.Jobs {
		metadata := Metadata{HealthObjective: policy.HealthObjectives[job.Type], Labels: append([]string{}, policy.Labels...), Annotations: map[string]string{}}
		metadata.Importance.RequiredPresubmitIDs = requiredPresubmits[job.ID]
		metadata.Importance.PresubmitBranchScope = presubmitBranchScope(job, policy)
		for key, value := range policy.Annotations {
			metadata.Annotations[key] = value
		}
		for _, group := range policy.Groups {
			if matches(job, group) {
				metadata.Importance.Group = group.ID
				metadata.Importance.MatchedRules = []string{group.ID}
				break
			}
		}
		for _, version := range job.Versions {
			if _, _, known := releaseconfig.ParseRelease(version); known && (compareRelease(version, metadata.Importance.ResolvedRelease) < 0 || (compareRelease(version, metadata.Importance.ResolvedRelease) == 0 && version < metadata.Importance.ResolvedRelease)) {
				metadata.Importance.ResolvedRelease = version
			}
		}
		metadata.Importance.ScopeRelease = metadata.Importance.ResolvedRelease
		if job.Presubmit != nil {
			if _, _, known := releaseconfig.ParseRelease(job.Presubmit.TargetRelease); known && compareRelease(job.Presubmit.TargetRelease, metadata.Importance.ScopeRelease) < 0 {
				metadata.Importance.ScopeRelease = job.Presubmit.TargetRelease
			}
			if metadata.Importance.PresubmitBranchScope == "development" {
				metadata.Importance.ResolvedRelease = policy.DevelopmentRelease
				if compareRelease(policy.DevelopmentRelease, metadata.Importance.ScopeRelease) < 0 {
					metadata.Importance.ScopeRelease = policy.DevelopmentRelease
				}
			} else if _, _, known := releaseconfig.ParseRelease(job.Presubmit.TargetRelease); known {
				metadata.Importance.ResolvedRelease = job.Presubmit.TargetRelease
			}
		}
		metadata.Importance.ReleaseScope = "in-horizon"
		if metadata.Importance.ScopeRelease == "" {
			metadata.Importance.ReleaseScope = "unknown"
		} else if compareRelease(metadata.Importance.ScopeRelease, policy.DevelopmentRelease) < 0 {
			metadata.Importance.ReleaseScope = "upcoming"
		}

		for _, platform := range job.Platforms {
			cloud := platform
			if alias, found := policy.CloudAliases[cloud]; found {
				cloud = alias
			}
			if cloudRank(cloud) < cloudRank(metadata.Importance.ResolvedCloud) {
				metadata.Importance.ResolvedCloud = cloud
			}
		}
		override, overridden := policy.Overrides[job.ID]
		if overridden && override.Group != "" {
			metadata.Importance.Group = override.Group
		}
		group := policy.Groups[groupOrder[metadata.Importance.Group]]
		if objective, found := group.HealthObjectives[job.Type]; found {
			metadata.HealthObjective = objective
		}
		if overridden {
			metadata.OverrideRationale = override.Rationale
			metadata.Importance.MatchedRules = append(metadata.Importance.MatchedRules, "job-override")
			if override.HealthObjective != nil {
				metadata.HealthObjective = *override.HealthObjective
			}
			metadata.Labels = append(metadata.Labels, override.Labels...)
			for key, value := range override.Annotations {
				metadata.Annotations[key] = value
			}
		}
		sort.Strings(metadata.Labels)
		metadata.Labels = slices.Compact(metadata.Labels)
		catalog.Jobs = append(catalog.Jobs, Entry{JobID: job.ID, TeamMetadata: metadata})
	}
	sort.Slice(catalog.Jobs, func(i, j int) bool {
		left, right := catalog.Jobs[i], catalog.Jobs[j]
		a, b := left.TeamMetadata.Importance, right.TeamMetadata.Importance
		if (a.ReleaseScope == "upcoming") != (b.ReleaseScope == "upcoming") {
			return a.ReleaseScope != "upcoming"
		}
		if (a.PresubmitBranchScope == "prospective") != (b.PresubmitBranchScope == "prospective") {
			return a.PresubmitBranchScope != "prospective"
		}
		if groupOrder[a.Group] != groupOrder[b.Group] {
			return groupOrder[a.Group] < groupOrder[b.Group]
		}
		for _, dimension := range policy.Dimensions {
			if dimension == "development-presubmit" {
				leftDevelopment := a.PresubmitBranchScope == "development"
				rightDevelopment := b.PresubmitBranchScope == "development"
				if leftDevelopment != rightDevelopment {
					return leftDevelopment
				}
			}
			if dimension == "required-presubmit-periodic" {
				leftRequired := len(a.RequiredPresubmitIDs) > 0
				rightRequired := len(b.RequiredPresubmitIDs) > 0
				if leftRequired != rightRequired {
					return leftRequired
				}
			}
			if dimension == "non-e2e-presubmit" {
				leftJob, rightJob := index[left.JobID], index[right.JobID]
				leftNonE2E := leftJob.Type == jobregistry.JobTypePresubmit && leftJob.E2EFramework == jobregistry.E2EFrameworkNone
				rightNonE2E := rightJob.Type == jobregistry.JobTypePresubmit && rightJob.E2EFramework == jobregistry.E2EFrameworkNone
				if leftNonE2E != rightNonE2E {
					return leftNonE2E
				}
			}
			if dimension == "release" {
				if comparison := compareRelease(a.ResolvedRelease, b.ResolvedRelease); comparison != 0 {
					return comparison < 0
				}
			}
			if dimension == "cloud" && cloudRank(a.ResolvedCloud) != cloudRank(b.ResolvedCloud) {
				return cloudRank(a.ResolvedCloud) < cloudRank(b.ResolvedCloud)
			}
		}
		return left.JobID < right.JobID
	})
	for i := range catalog.Jobs {
		catalog.Jobs[i].TeamMetadata.Importance.Rank = i + 1
	}
	return catalog, nil
}

func LoadFile(path string, registry *jobregistry.Registry) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("catalog must contain exactly one JSON value")
	}
	if err := catalog.Validate(registry); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (catalog *Catalog) Validate(registry *jobregistry.Registry) error {
	expected, err := Evaluate(registry, catalog.Policy)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(catalog, expected) {
		return fmt.Errorf("job policy catalog is not the canonical evaluation of its registry, policy, and algorithm")
	}
	return nil
}
