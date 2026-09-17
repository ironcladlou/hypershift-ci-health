package jobregistry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// CurrentAPIVersion identifies the schema emitted by the registry generator.
	CurrentAPIVersion           = "job-registry/v7"
	releaseRepository           = "openshift/release"
	releaseMainURL              = "https://github.com/openshift/release/blob/main/"
	defaultSippyURL             = "https://sippy.dptools.openshift.org"
	defaultSippyStreamURL       = "https://sippy-auth.dptools.openshift.org"
	defaultProwURL              = "https://prow.ci.openshift.org"
	defaultReleaseStatusURL     = "https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com"
	sippyPresubmits             = "Presubmits"
	jobsRoot                    = "ci-operator/jobs"
	hypershiftJobsDir           = "ci-operator/jobs/openshift/hypershift"
	releaseControllerConfigsDir = "core-services/release-controller/_releases"
)

var (
	releaseVersionRE  = regexp.MustCompile(`(?:^|-)release-(\d+\.\d+)(?:-|$)`)
	embeddedVersionRE = regexp.MustCompile(`(?:^|-)([4-9])-(\d{1,2})(?:-|$)`)
	conformanceRE     = regexp.MustCompile(`(?:^|/)hypershift-.+-conformance(?:-|$)`)
	releaseStreamRE   = regexp.MustCompile(`^(\d+\.\d+)\.0-0\.(ci|nightly)(?:-(arm64|ppc64le|s390x|multi))?$`)
)

type releaseControllerConfig struct {
	Name      string                                   `json:"name"`
	EndOfLife bool                                     `json:"endOfLife"`
	Verify    map[string]releaseControllerVerification `json:"verify"`
}

type releaseControllerVerification struct {
	Disabled          bool                                `json:"disabled"`
	Optional          bool                                `json:"optional"`
	Async             bool                                `json:"async"`
	Upgrade           bool                                `json:"upgrade"`
	UpgradeFrom       string                              `json:"upgradeFrom"`
	ProwJob           *releaseControllerProwJob           `json:"prowJob"`
	MaxRetries        int                                 `json:"maxRetries"`
	AggregatedProwJob *releaseControllerAggregatedProwJob `json:"aggregatedProwJob"`
	MultiJobAnalysis  bool                                `json:"multiJobAnalysis"`
}

type releaseControllerProwJob struct {
	Name string `json:"name"`
}

type releaseControllerAggregatedProwJob struct {
	ProwJob          *releaseControllerProwJob `json:"prowJob"`
	AnalysisJobCount int                       `json:"analysisJobCount"`
}

type prowConfig struct {
	Periodics  []prowJob            `yaml:"periodics"`
	Presubmits map[string][]prowJob `yaml:"presubmits"`
}

type prowJob struct {
	Name              string            `yaml:"name"`
	Context           string            `yaml:"context"`
	Labels            map[string]string `yaml:"labels"`
	Optional          bool              `yaml:"optional"`
	AlwaysRun         bool              `yaml:"always_run"`
	Branches          []string          `yaml:"branches"`
	SkipBranches      []string          `yaml:"skip_branches"`
	RunIfChanged      string            `yaml:"run_if_changed"`
	SkipIfOnlyChanged string            `yaml:"skip_if_only_changed"`
	ExtraRefs         []prowRef         `yaml:"extra_refs"`
	Spec              struct {
		Containers []struct {
			Args []string `yaml:"args"`
		} `yaml:"containers"`
	} `yaml:"spec"`
}

type prowRef struct {
	Org  string `yaml:"org"`
	Repo string `yaml:"repo"`
}

// Options controls deterministic navigation links added during discovery.
// Empty values use the public OpenShift service defaults.
type Options struct {
	SippyBaseURL         string
	SippyStreamBaseURL   string
	ProwBaseURL          string
	ReleaseStatusBaseURL string
}

// Discover builds a registry from an openshift/release checkout.
func Discover(releaseDir string, options Options) (Registry, error) {
	if err := defaultPresubmitPolicy.validate(); err != nil {
		return Registry{}, fmt.Errorf("validate presubmit generator policy: %w", err)
	}
	if options.SippyBaseURL == "" {
		options.SippyBaseURL = defaultSippyURL
	}
	if options.SippyStreamBaseURL == "" {
		options.SippyStreamBaseURL = defaultSippyStreamURL
	}
	if options.ProwBaseURL == "" {
		options.ProwBaseURL = defaultProwURL
	}
	if options.ReleaseStatusBaseURL == "" {
		options.ReleaseStatusBaseURL = defaultReleaseStatusURL
	}

	registry, err := discover(releaseDir)
	if err != nil {
		return Registry{}, fmt.Errorf("discover jobs: %w", err)
	}
	populateSippyURLs(&registry, options.SippyBaseURL)
	populateProwJobHistoryURLs(&registry, options.ProwBaseURL)
	if err := populateReleaseController(&registry, releaseDir, options.SippyStreamBaseURL, options.ReleaseStatusBaseURL); err != nil {
		return Registry{}, fmt.Errorf("discover release-controller participation: %w", err)
	}
	if err := populatePeriodicCounterparts(&registry, defaultPresubmitPolicy); err != nil {
		return Registry{}, fmt.Errorf("discover periodic counterparts: %w", err)
	}
	populateSippyIngestion(&registry, defaultPresubmitPolicy)
	if err := registry.Validate(); err != nil {
		return Registry{}, fmt.Errorf("validate decorated registry: %w", err)
	}
	return registry, nil
}

func populateReleaseController(registry *Registry, releaseDir, sippyBaseURL, releaseStatusBaseURL string) error {
	root := filepath.Join(releaseDir, releaseControllerConfigsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	jobsByName := make(map[string]*Job, len(registry.Jobs))
	for i := range registry.Jobs {
		jobsByName[registry.Jobs[i].Name] = &registry.Jobs[i]
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var config releaseControllerConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		verificationNames := make([]string, 0, len(config.Verify))
		for name := range config.Verify {
			verificationNames = append(verificationNames, name)
		}
		sort.Strings(verificationNames)
		for _, verificationName := range verificationNames {
			verification := config.Verify[verificationName]
			if verification.ProwJob == nil {
				continue
			}
			job := jobsByName[verification.ProwJob.Name]
			if job == nil {
				continue
			}

			rel, err := filepath.Rel(releaseDir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			job.ReleaseController = append(job.ReleaseController, releaseControllerParticipation(
				config,
				verificationName,
				verification,
				Source{
					Repository:  releaseRepository,
					Path:        rel,
					URL:         releaseMainURL + rel,
					Declaration: "verify." + verificationName,
				},
				sippyBaseURL,
				releaseStatusBaseURL,
			))
		}
	}

	for i := range registry.Jobs {
		sort.Slice(registry.Jobs[i].ReleaseController, func(a, b int) bool {
			left := registry.Jobs[i].ReleaseController[a]
			right := registry.Jobs[i].ReleaseController[b]
			if left.Stream.Name != right.Stream.Name {
				return left.Stream.Name < right.Stream.Name
			}
			return left.Verification.Name < right.Verification.Name
		})
	}
	return nil
}

func releaseControllerParticipation(config releaseControllerConfig, verificationName string, verification releaseControllerVerification, source Source, sippyBaseURL, releaseStatusBaseURL string) ReleaseControllerParticipation {
	release, kind, architecture, recognized := parseReleaseStream(config.Name)
	var sippyURL *string
	if recognized {
		link := strings.TrimRight(sippyBaseURL, "/") + "/sippy-ng/release/" + url.PathEscape(release) + "/streams/" + url.PathEscape(architecture) + "/" + url.PathEscape(kind) + "/overview"
		sippyURL = &link
	}

	var aggregated *ReleaseControllerAggregation
	if verification.AggregatedProwJob != nil {
		aggregated = &ReleaseControllerAggregation{AnalysisJobCount: verification.AggregatedProwJob.AnalysisJobCount}
		if verification.AggregatedProwJob.ProwJob != nil {
			aggregated.ProwJobName = verification.AggregatedProwJob.ProwJob.Name
		}
	}

	return ReleaseControllerParticipation{
		Stream: ReleaseControllerStream{
			Name:             config.Name,
			Release:          release,
			Kind:             ReleaseControllerStreamKind(kind),
			Architecture:     architecture,
			EndOfLife:        config.EndOfLife,
			SippyURL:         sippyURL,
			ReleaseStatusURL: releaseStatusURL(releaseStatusBaseURL, config.Name, architecture),
		},
		Verification: ReleaseControllerVerification{
			Name:             verificationName,
			Role:             releaseControllerRole(verification),
			Optional:         verification.Optional,
			Disabled:         verification.Disabled,
			Async:            verification.Async,
			Upgrade:          verification.Upgrade,
			UpgradeFrom:      verification.UpgradeFrom,
			MaxRetries:       verification.MaxRetries,
			Aggregated:       aggregated,
			MultiJobAnalysis: verification.MultiJobAnalysis,
		},
		Source: source,
	}
}

func releaseStatusURL(baseURL, stream, architecture string) string {
	// The central status application exposes amd64 streams. Other recognized
	// architectures are served by their architecture-specific release
	// controller. A caller-provided base URL intentionally overrides this
	// default routing for every architecture.
	if baseURL == defaultReleaseStatusURL && architecture != "" && architecture != "amd64" {
		baseURL = "https://" + architecture + ".ocp.releases.ci.openshift.org"
	}
	return strings.TrimRight(baseURL, "/") + "/releasestream/" + url.PathEscape(stream)
}

func parseReleaseStream(name string) (release, kind, architecture string, ok bool) {
	match := releaseStreamRE.FindStringSubmatch(name)
	if match == nil {
		return "", "", "", false
	}
	architecture = match[3]
	if architecture == "" {
		architecture = "amd64"
	}
	return match[1], match[2], architecture, true
}

func releaseControllerRole(verification releaseControllerVerification) ReleaseControllerRole {
	switch {
	case verification.Disabled:
		return ReleaseControllerRoleDisabled
	case verification.Async:
		return ReleaseControllerRoleAsync
	case verification.Optional:
		return ReleaseControllerRoleInforming
	default:
		return ReleaseControllerRoleBlocking
	}
}

func populateProwJobHistoryURLs(registry *Registry, baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/") + "/job-history/gs/test-platform-results/"
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type == JobTypePresubmit {
			job.ProwJobHistoryURL = baseURL + "pr-logs/directory/" + url.PathEscape(job.Name)
			continue
		}
		job.ProwJobHistoryURL = baseURL + "logs/" + url.PathEscape(job.Name)
	}
}

func populateSippyURLs(registry *Registry, baseURL string) {
	for i := range registry.Jobs {
		job := &registry.Jobs[i]
		if job.Type == JobTypePresubmit {
			link := sippyJobURL(baseURL, sippyPresubmits, job.Name, true)
			job.SippyURL = &link
			continue
		}

		match := releaseVersionRE.FindStringSubmatch(job.Name)
		if match == nil {
			continue
		}
		link := sippyJobURL(baseURL, match[1], job.Name, false)
		job.SippyURL = &link
	}
}

func sippyJobURL(baseURL, release, name string, analysis bool) string {
	filter := `{"items":[{"columnField":"name","operatorValue":"equals","value":` + strconv.Quote(name) + `}]}`
	query := url.Values{"filters": []string{filter}}
	path := strings.TrimRight(baseURL, "/") + "/sippy-ng/jobs/" + url.PathEscape(release)
	if analysis {
		path += "/analysis"
	}
	return path + "?" + query.Encode()
}

func discover(releaseDir string) (Registry, error) {
	root := filepath.Join(releaseDir, jobsRoot)
	if info, err := os.Stat(root); err != nil {
		return Registry{}, err
	} else if !info.IsDir() {
		return Registry{}, fmt.Errorf("%s is not a directory", root)
	}

	registry := Registry{
		APIVersion: CurrentAPIVersion,
		Jobs:       []Job{},
	}
	seen := map[string]Source{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isJobConfig(entry.Name()) {
			return nil
		}

		rel, err := filepath.Rel(releaseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		isHypershiftConfig := strings.HasPrefix(rel, hypershiftJobsDir+"/")

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Most of openshift/release is irrelevant. This textual check keeps the
		// full-tree scan cheap; parsed jobs are still checked structurally below.
		if !isHypershiftConfig && !bytes.Contains(data, []byte("hypershift-")) {
			return nil
		}

		var config prowConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		source := Source{Repository: releaseRepository, Path: rel, URL: releaseMainURL + rel}
		for _, candidate := range config.Periodics {
			if isHypershiftConfig || isHypershiftConformance(candidate) {
				if err := addJob(&registry, seen, candidate, JobTypePeriodic, repositoryFromJob(candidate), source); err != nil {
					return err
				}
			}
		}
		for repository, candidates := range config.Presubmits {
			for _, candidate := range candidates {
				if isHypershiftConfig || isHypershiftConformance(candidate) {
					if err := addJob(&registry, seen, candidate, JobTypePresubmit, repository, source); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return Registry{}, err
	}

	sort.Slice(registry.Jobs, func(i, j int) bool { return registry.Jobs[i].ID < registry.Jobs[j].ID })
	return registry, nil
}

func isJobConfig(name string) bool {
	return strings.HasSuffix(name, "-periodics.yaml") || strings.HasSuffix(name, "-presubmits.yaml")
}

func isHypershiftConformance(job prowJob) bool {
	if conformanceRE.MatchString(job.Context) {
		return true
	}
	for _, container := range job.Spec.Containers {
		for _, arg := range container.Args {
			if strings.HasPrefix(arg, "--target=") && conformanceRE.MatchString(strings.TrimPrefix(arg, "--target=")) {
				return true
			}
		}
	}
	return false
}

func addJob(registry *Registry, seen map[string]Source, candidate prowJob, jobType JobType, repository string, source Source) error {
	if candidate.Name == "" {
		return errors.New("encountered a job without a name in " + source.Path)
	}
	if prior, exists := seen[candidate.Name]; exists {
		return fmt.Errorf("duplicate job identity %q in %s and %s", candidate.Name, prior.Path, source.Path)
	}
	seen[candidate.Name] = source
	job := Job{
		ID:                candidate.Name,
		Name:              candidate.Name,
		Repository:        repository,
		Context:           candidate.Context,
		Type:              jobType,
		Source:            source,
		Versions:          versions(candidate),
		Platforms:         platforms(candidate),
		E2EFramework:      framework(candidate),
		Variant:           candidate.Labels["ci-operator.openshift.io/variant"],
		ReleaseController: []ReleaseControllerParticipation{},
		SippyURL:          nil, // Populated after all jobs are discovered.
	}
	if jobType == JobTypePresubmit {
		job.Presubmit = &Presubmit{
			Required:             !candidate.Optional,
			AlwaysRun:            candidate.AlwaysRun,
			Branches:             nonNil(candidate.Branches),
			SkipBranches:         nonNil(candidate.SkipBranches),
			RunIfChanged:         candidate.RunIfChanged,
			SkipIfOnlyChanged:    candidate.SkipIfOnlyChanged,
			PeriodicCounterparts: []PeriodicCounterpart{},
		}
	}
	registry.Jobs = append(registry.Jobs, job)
	return nil
}

func repositoryFromJob(job prowJob) string {
	if len(job.ExtraRefs) > 0 && job.ExtraRefs[0].Org != "" && job.ExtraRefs[0].Repo != "" {
		return job.ExtraRefs[0].Org + "/" + job.ExtraRefs[0].Repo
	}
	org := job.Labels["ci-operator.openshift.io/refs.org"]
	repo := job.Labels["ci-operator.openshift.io/refs.repo"]
	if org == "" || repo == "" {
		return ""
	}
	return org + "/" + repo
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func versions(job prowJob) []string {
	values := map[string]struct{}{}
	if match := releaseVersionRE.FindStringSubmatch(job.Name); match != nil {
		values[match[1]] = struct{}{}
	}
	for _, match := range embeddedVersionRE.FindAllStringSubmatch(job.Name, -1) {
		values[match[1]+"."+match[2]] = struct{}{}
	}
	return sortedKeys(values)
}

func platforms(job prowJob) []string {
	text := strings.ToLower(job.Name + " " + job.Context)
	for key, value := range job.Labels {
		text += " " + strings.ToLower(key) + "=" + strings.ToLower(value)
	}
	values := map[string]struct{}{}
	rules := []struct {
		name    string
		needles []string
	}{
		{"aro", []string{"-aks", "hypershift-aks", "aro-hcp", "e2e-aro-"}},
		{"kubevirt", []string{"kubevirt"}},
		{"openstack", []string{"openstack"}},
		{"powervs", []string{"powervs"}},
		{"ibmcloud", []string{"ibmcloud"}},
		{"aws", []string{"-aws", "hypershift-aws"}},
		{"azure", []string{"-azure", "hypershift-azure", "azure4"}},
		{"gcp", []string{"-gcp", "-gke", "hypershift-gcp"}},
		{"agent", []string{"-mce-", "e2e-agent-", "hypershift-mce-agent"}},
		{"baremetal", []string{"-metal-", "baremetalds", "equinix"}},
	}
	for _, rule := range rules {
		for _, needle := range rule.needles {
			if strings.Contains(text, needle) {
				values[rule.name] = struct{}{}
				break
			}
		}
	}
	return sortedKeys(values)
}

func framework(job prowJob) E2EFramework {
	text := strings.ToLower(job.Name + " " + job.Context)
	if strings.Contains(text, "e2e-v2") {
		return E2EFrameworkV2
	}
	if strings.Contains(text, "e2e") || strings.Contains(text, "conformance") {
		return E2EFrameworkV1
	}
	return E2EFrameworkNone
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
