package healthreport

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/sippy"
)

type WindowConfig struct {
	SparkSlotHours int
	SparkSlots     int
	CurrentDays    int
}

var WindowConfigs = map[string]WindowConfig{
	"1w": {SparkSlotHours: 6, SparkSlots: 28, CurrentDays: 7},
	"2w": {SparkSlotHours: 12, SparkSlots: 28, CurrentDays: 14},
	"1m": {SparkSlotHours: 24, SparkSlots: 30, CurrentDays: 30},
}

type rawData struct {
	analyses           map[analysisKey]*analysisData
	recentFailures     []sippy.RecentFailure
	componentReadiness []componentReadinessConfig
}

type analysisData struct {
	ByPeriod map[string]sippy.AnalysisPeriod
}

type analysisSummary struct {
	CurrentPassPercentage  float64
	PreviousPassPercentage float64
	NetImprovement         float64
	CurrentRuns            int
	CurrentFails           int
	PreviousRuns           int
}

type periodicConfig struct {
	Job         *jobregistry.Job
	Counterpart reportplan.PeriodicReference
}

type presubmitConfig struct {
	Job       *jobregistry.Job
	Role      string
	Periodics []periodicConfig
}

type payloadConfig struct {
	Job            *jobregistry.Job
	Release        string
	Participations []jobregistry.ReleaseControllerParticipation
}

type componentReadinessConfig struct {
	Membership sippy.ComponentReadinessMembership
	Job        *jobregistry.Job
}

type reportInputs struct {
	Presubmits  []presubmitConfig
	PayloadJobs []payloadConfig
}

// Evaluate is a pure join of registry facts, planned participation, and Sippy
// observations. It performs no I/O and does not mutate any input.
func Evaluate(registry *jobregistry.Registry, plan *reportplan.Plan, observation *sippy.Observation) (*Report, error) {
	if err := plan.Validate(registry); err != nil {
		return nil, err
	}
	planDigest, err := reportplan.Digest(plan)
	if err != nil {
		return nil, err
	}
	if err := observation.Validate(planDigest); err != nil {
		return nil, err
	}
	observationDigest, err := sippy.ObservationDigest(observation)
	if err != nil {
		return nil, err
	}
	index := registry.Index()
	nameIndex := make(map[string]*jobregistry.Job, len(registry.Jobs))
	for i := range registry.Jobs {
		nameIndex[registry.Jobs[i].Name] = &registry.Jobs[i]
	}
	inputs := reportInputs{Presubmits: []presubmitConfig{}, PayloadJobs: []payloadConfig{}}
	for _, entry := range plan.Presubmits {
		configured := presubmitConfig{Job: index[entry.JobID], Role: entry.Role, Periodics: []periodicConfig{}}
		for _, periodic := range entry.Periodics {
			configured.Periodics = append(configured.Periodics, periodicConfig{Job: index[periodic.JobID], Counterpart: periodic})
		}
		inputs.Presubmits = append(inputs.Presubmits, configured)
	}
	for _, entry := range plan.PayloadJobs {
		inputs.PayloadJobs = append(inputs.PayloadJobs, payloadConfig{Job: index[entry.JobID], Release: entry.Release, Participations: entry.Participations})
	}
	analyses := make(map[analysisKey]*analysisData, len(observation.Analyses))
	for _, analysis := range observation.Analyses {
		key := analysisKey{release: analysis.Target.Release, jobID: analysis.Target.ProwJobName}
		analyses[key] = &analysisData{ByPeriod: analysis.ByPeriod}
	}
	components := make([]componentReadinessConfig, 0, len(observation.Memberships))
	releaseOrder := make(map[string]int, len(plan.Releases))
	for index, release := range plan.Releases {
		releaseOrder[release] = index
	}
	for _, membership := range observation.Memberships {
		if membership.Tier != sippy.JobTierStandard || !slices.Contains(plan.Releases, membership.Release) {
			continue
		}
		components = append(components, componentReadinessConfig{Membership: membership, Job: nameIndex[membership.ProwJobName]})
	}
	sort.Slice(components, func(i, j int) bool {
		if components[i].Membership.Release != components[j].Membership.Release {
			return releaseOrder[components[i].Membership.Release] > releaseOrder[components[j].Membership.Release]
		}
		return components[i].Membership.ProwJobName < components[j].Membership.ProwJobName
	})
	raw := &rawData{analyses: analyses, recentFailures: observation.RecentFailures, componentReadiness: components}
	collection := make([]CollectionResult, 0, len(observation.Collection))
	for _, result := range observation.Collection {
		collection = append(collection, CollectionResult{Kind: result.Kind, Release: result.Release, ProwJobName: result.ProwJobName, Error: result.Error})
	}
	report := &Report{APIVersion: CurrentAPIVersion, RegistryDigest: plan.RegistryDigest, PlanDigest: planDigest, ObservationDigest: observationDigest, GeneratedAt: observation.ObservedAt, ObservationStartedAt: observation.StartedAt, Complete: observation.Complete, Collection: collection, Windows: map[string]*WindowData{}, Platforms: collectedPlatforms(plan.Platforms, components), Releases: slices.Clone(plan.Releases)}
	for windowKey := range WindowConfigs {
		report.Windows[windowKey] = transformWindow(raw, windowKey, observation.ObservedAt, inputs)
	}
	return report, nil
}

func collectedPlatforms(static []string, components []componentReadinessConfig) []string {
	seen := map[string]bool{}
	for _, platform := range static {
		seen[platform] = true
	}
	for _, component := range components {
		if component.Job != nil {
			for _, platform := range component.Job.Platforms {
				seen[platform] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for platform := range seen {
		result = append(result, platform)
	}
	sort.Strings(result)
	return result
}

type analysisKey struct {
	release string
	jobID   string
}

func slotKey(t time.Time, slotHours int) string {
	slotHour := (t.Hour() / slotHours) * slotHours
	return fmt.Sprintf("%04d-%02d-%02d %02d:00", t.Year(), int(t.Month()), t.Day(), slotHour)
}

func slotKeys(now time.Time, win WindowConfig) []string {
	keys := make([]string, win.SparkSlots)
	for i := win.SparkSlots - 1; i >= 0; i-- {
		t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
		slotBase := (t.Hour() / win.SparkSlotHours) * win.SparkSlotHours
		t = time.Date(t.Year(), t.Month(), t.Day(), slotBase, 0, 0, 0, time.UTC)
		t = t.Add(-time.Duration(i) * time.Duration(win.SparkSlotHours) * time.Hour)
		keys[win.SparkSlots-1-i] = slotKey(t, win.SparkSlotHours)
	}
	return keys
}

func analysisSparkline(analysis *analysisData, win WindowConfig, now time.Time) map[string]*SparklineSlot {
	if analysis == nil {
		return nil
	}
	cutoff := now.Add(-time.Duration(win.SparkSlots*win.SparkSlotHours) * time.Hour)
	data := make(map[string]*SparklineSlot)
	for period, result := range analysis.ByPeriod {
		t, err := time.ParseInLocation("2006-01-02 15:00", period, time.UTC)
		if err != nil || t.Before(cutoff) {
			continue
		}
		key := slotKey(t, win.SparkSlotHours)
		slot, ok := data[key]
		if !ok {
			slot = &SparklineSlot{}
			data[key] = slot
		}
		slot.TotalRuns += result.TotalRuns
		slot.Passes += result.ResultCount["S"]
		slot.TestFailures += result.ResultCount["F"]
		slot.InfraFailures += result.ResultCount["n"] + result.ResultCount["N"]
	}
	return data
}

func summarizeAnalysis(analysis *analysisData, windowKey string, now time.Time) *analysisSummary {
	if analysis == nil {
		return nil
	}
	currentDuration := time.Duration(WindowConfigs[windowKey].CurrentDays) * 24 * time.Hour
	currentStart := now.Add(-currentDuration)
	previousStart := currentStart.Add(-currentDuration)

	var currentRuns, currentPasses, previousRuns, previousPasses int
	for period, result := range analysis.ByPeriod {
		t, err := time.ParseInLocation("2006-01-02 15:00", period, time.UTC)
		if err != nil || t.After(now) || t.Before(previousStart) {
			continue
		}
		if t.Before(currentStart) {
			previousRuns += result.TotalRuns
			previousPasses += result.ResultCount["S"]
		} else {
			currentRuns += result.TotalRuns
			currentPasses += result.ResultCount["S"]
		}
	}

	summary := &analysisSummary{
		CurrentRuns:  currentRuns,
		CurrentFails: currentRuns - currentPasses,
		PreviousRuns: previousRuns,
	}
	if currentRuns > 0 {
		summary.CurrentPassPercentage = 100 * float64(currentPasses) / float64(currentRuns)
	}
	if previousRuns > 0 {
		summary.PreviousPassPercentage = 100 * float64(previousPasses) / float64(previousRuns)
	}
	summary.NetImprovement = summary.CurrentPassPercentage - summary.PreviousPassPercentage
	return summary
}

type resultCounts struct {
	testFails  int
	infraFails int
	passes     int
	sparkRuns  int
}

func countResultTypes(sparkline map[string]*SparklineSlot) resultCounts {
	var c resultCounts
	for _, slot := range sparkline {
		c.passes += slot.Passes
		c.testFails += slot.TestFailures
		c.infraFails += slot.InfraFailures
		c.sparkRuns += slot.TotalRuns
	}
	return c
}

func computeCorrelation(preSparkline, perSparkline map[string]*SparklineSlot, now time.Time, win WindowConfig) *Correlation {
	if preSparkline == nil || perSparkline == nil {
		return nil
	}
	keys := slotKeys(now, win)
	var preFails, correlated int
	var indices []int

	for idx, k := range keys {
		pre := preSparkline[k]
		if pre == nil || pre.TotalRuns == 0 {
			continue
		}
		prePassRate := float64(pre.Passes) / float64(pre.TotalRuns)
		if prePassRate >= 0.5 {
			continue
		}
		preFails++

		per := perSparkline[k]
		if per == nil || per.TotalRuns == 0 {
			continue
		}
		perPassRate := float64(per.Passes) / float64(per.TotalRuns)
		if perPassRate < 0.5 {
			correlated++
			indices = append(indices, idx)
		}
	}

	if preFails == 0 {
		return nil
	}
	return &Correlation{
		Correlated: correlated,
		PreFails:   preFails,
		Indices:    indices,
	}
}

func buildPeriodicHealth(key analysisKey, id, name, prow, release, label, relationshipSource, relationshipVerification, relationshipRationale, prowJobHistoryURL string, periodicMap map[analysisKey]*analysisSummary, sparklines map[analysisKey]map[string]*SparklineSlot, slots []string) PeriodicJobHealth {
	d := periodicMap[key]
	sparkline := sparklines[key]
	counts := countResultTypes(sparkline)
	health := PeriodicJobHealth{
		ID:                       id,
		Name:                     name,
		Prow:                     prow,
		Release:                  release,
		Label:                    label,
		RelationshipSource:       relationshipSource,
		RelationshipVerification: relationshipVerification,
		RelationshipRationale:    relationshipRationale,
		TestFails:                counts.testFails,
		InfraFails:               counts.infraFails,
		SparkRuns:                counts.sparkRuns,
		Sparkline:                orderedSparkline(sparkline, slots),
		ProwJobHistoryURL:        prowJobHistoryURL,
	}
	if d != nil {
		health.Rate = &d.CurrentPassPercentage
		health.Prev = &d.PreviousPassPercentage
		health.PrevRuns = d.PreviousRuns
		health.Trend = &d.NetImprovement
		health.Runs = d.CurrentRuns
		health.Fails = d.CurrentFails
	}
	return health
}

func orderedSparkline(sparkline map[string]*SparklineSlot, slots []string) []*SparklineSlot {
	if sparkline == nil {
		return nil
	}
	result := make([]*SparklineSlot, len(slots))
	for index, slot := range slots {
		result[index] = sparkline[slot]
	}
	return result
}

func transformWindow(raw *rawData, windowKey string, now time.Time, inputs reportInputs) *WindowData {
	win := WindowConfigs[windowKey]
	slots := slotKeys(now, win)

	summaries := make(map[analysisKey]*analysisSummary, len(raw.analyses))
	sparklines := make(map[analysisKey]map[string]*SparklineSlot, len(raw.analyses))
	for key, analysis := range raw.analyses {
		summaries[key] = summarizeAnalysis(analysis, windowKey, now)
		sparklines[key] = analysisSparkline(analysis, win, now)
	}

	blockingProwNames := make(map[string]bool)
	for _, bj := range inputs.Presubmits {
		blockingProwNames[bj.Job.Name] = true
	}

	var jobHealths []JobHealth
	for _, cfg := range inputs.Presubmits {
		key := analysisKey{release: "Presubmits", jobID: cfg.Job.Name}
		d := summaries[key]

		var periodics []PeriodicJobHealth
		for _, cfgPer := range cfg.Periodics {
			periodicKey := analysisKey{release: cfgPer.Counterpart.TestedRelease, jobID: cfgPer.Job.Name}
			periodics = append(periodics, buildPeriodicHealth(
				periodicKey,
				cfgPer.Job.ID, displayName(cfgPer.Job.Name), cfgPer.Job.Name, cfgPer.Counterpart.TestedRelease, cfgPer.Counterpart.TestedRelease,
				string(cfgPer.Counterpart.Source), string(cfgPer.Counterpart.Verification), cfgPer.Counterpart.Rationale, cfgPer.Job.ProwJobHistoryURL,
				summaries, sparklines, slots,
			))
		}

		var correlation *Correlation
		if len(cfg.Periodics) > 0 {
			periodicKey := analysisKey{release: cfg.Periodics[0].Counterpart.TestedRelease, jobID: cfg.Periodics[0].Job.Name}
			correlation = computeCorrelation(
				sparklines[key],
				sparklines[periodicKey],
				now, win,
			)
		}

		preSparkline := sparklines[key]
		preCounts := countResultTypes(preSparkline)

		jh := JobHealth{
			ID:                    cfg.Job.ID,
			Name:                  displayName(cfg.Job.Name),
			Prow:                  cfg.Job.Name,
			TargetBranch:          cfg.Job.Presubmit.TargetBranch,
			TargetRelease:         cfg.Job.Presubmit.TargetRelease,
			Platforms:             cfg.Job.Platforms,
			Role:                  string(cfg.Role),
			RoleLabel:             roleLabel(cfg.Role, cfg.Job.Presubmit.TargetRelease),
			TestFails:             preCounts.testFails,
			InfraFails:            preCounts.infraFails,
			SparkRuns:             preCounts.sparkRuns,
			Periodics:             periodics,
			Sparkline:             orderedSparkline(preSparkline, slots),
			Correlation:           correlation,
			SippyIngestionEnabled: cfg.Job.Presubmit.SippyIngestion.Enabled,
			SippyIngestionBasis:   cfg.Job.Presubmit.SippyIngestion.Basis,
			ProwJobHistoryURL:     cfg.Job.ProwJobHistoryURL,
		}

		if d != nil {
			jh.Rate = d.CurrentPassPercentage
			jh.Prev = d.PreviousPassPercentage
			jh.PrevRuns = d.PreviousRuns
			jh.Trend = &d.NetImprovement
			jh.Runs = d.CurrentRuns
			jh.Fails = d.CurrentFails
		}

		jobHealths = append(jobHealths, jh)
	}

	payloadHealths := make([]PayloadBlockingJobHealth, 0, len(inputs.PayloadJobs))
	for _, cfg := range inputs.PayloadJobs {
		key := analysisKey{release: cfg.Release, jobID: cfg.Job.Name}
		health := buildPeriodicHealth(
			key,
			cfg.Job.ID, displayName(cfg.Job.Name), cfg.Job.Name, cfg.Release, "release payload",
			"", "", "", cfg.Job.ProwJobHistoryURL,
			summaries, sparklines, slots,
		)
		participations := make([]ReleasePayloadParticipation, 0, len(cfg.Participations))
		for _, participation := range cfg.Participations {
			participations = append(participations, ReleasePayloadParticipation{
				StreamName:       participation.Stream.Name,
				StreamKind:       string(participation.Stream.Kind),
				Architecture:     participation.Stream.Architecture,
				VerificationName: participation.Verification.Name,
				StreamSippyURL:   participation.Stream.SippyURL,
				ReleaseStatusURL: participation.Stream.ReleaseStatusURL,
			})
		}
		payloadHealths = append(payloadHealths, PayloadBlockingJobHealth{
			PeriodicJobHealth: health,
			Platforms:         cfg.Job.Platforms,
			Participations:    participations,
		})
	}

	componentReadinessHealths := make([]ComponentReadinessJobHealth, 0, len(raw.componentReadiness))
	for _, cfg := range raw.componentReadiness {
		membership := cfg.Membership
		key := analysisKey{release: membership.Release, jobID: membership.ProwJobName}
		var platforms []string
		var prowJobHistoryURL string
		if cfg.Job != nil {
			platforms = cfg.Job.Platforms
			prowJobHistoryURL = cfg.Job.ProwJobHistoryURL
		}
		health := buildPeriodicHealth(
			key,
			membership.ProwJobName, displayName(membership.ProwJobName), membership.ProwJobName, membership.Release, "component readiness",
			"", "", "", prowJobHistoryURL,
			summaries, sparklines, slots,
		)
		componentReadinessHealths = append(componentReadinessHealths, ComponentReadinessJobHealth{
			PeriodicJobHealth: health,
			Platforms:         platforms,
			RegistryMissing:   cfg.Job == nil,
		})
	}

	alerts := buildAlerts(raw.recentFailures, blockingProwNames)

	return &WindowData{
		SparklineSlots:         slots,
		Jobs:                   jobHealths,
		PayloadBlockingJobs:    payloadHealths,
		ComponentReadinessJobs: componentReadinessHealths,
		Alerts:                 alerts,
	}
}

func buildAlerts(failures []sippy.RecentFailure, blockingProwNames map[string]bool) []Alert {
	var alerts []Alert
	for _, f := range failures {
		var matching []string
		seen := make(map[string]bool)
		matchCount := 0
		for _, o := range f.Outputs {
			if !blockingProwNames[o.ProwJobName] {
				continue
			}
			matchCount++
			shortName := strings.TrimPrefix(o.ProwJobName, "pull-ci-openshift-hypershift-main-")
			if !seen[shortName] {
				seen[shortName] = true
				matching = append(matching, shortName)
			}
		}
		if matchCount == 0 {
			continue
		}
		alerts = append(alerts, Alert{
			TestName:     f.TestName,
			FailureCount: matchCount,
			Jobs:         matching,
		})
	}
	return alerts
}

func displayName(name string) string {
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-main-"); value != name {
		return value
	}
	if value := strings.TrimPrefix(name, "pull-ci-openshift-hypershift-release-"); value != name {
		if _, short, found := strings.Cut(value, "-"); found {
			return short
		}
	}
	if _, value, found := strings.Cut(name, "-periodics-"); found {
		return value
	}
	return name
}

func roleLabel(role, release string) string {
	if role == "future" {
		return fmt.Sprintf("Future (%s)", release)
	}
	if offset, found := strings.CutPrefix(role, "n-"); found {
		return fmt.Sprintf("N-%s (%s)", offset, release)
	}
	return fmt.Sprintf("Unknown (%s)", release)
}
