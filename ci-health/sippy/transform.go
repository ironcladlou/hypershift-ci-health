package sippy

import (
	"fmt"
	"strings"
	"time"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobs"
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
	analyses           map[string]*SippyJobAnalysisResponse
	recentFailures     []SippyTestFailure
	componentReadiness []jobs.ComponentReadinessJobConfig
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

func analysisSparkline(analysis *SippyJobAnalysisResponse, win WindowConfig, now time.Time) map[string]*SparklineSlot {
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
			slot = &SparklineSlot{ResultCount: make(map[string]int)}
			data[key] = slot
		}
		slot.TotalRuns += result.TotalRuns
		for resultCode, count := range result.ResultCount {
			slot.ResultCount[resultCode] += count
		}
	}
	return data
}

func summarizeAnalysis(analysis *SippyJobAnalysisResponse, windowKey string, now time.Time) *SippyJob {
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

	summary := &SippyJob{
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
		c.passes += slot.ResultCount["S"]
		c.testFails += slot.ResultCount["F"]
		c.infraFails += slot.ResultCount["n"] + slot.ResultCount["N"]
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
		prePassRate := float64(pre.ResultCount["S"]) / float64(pre.TotalRuns)
		if prePassRate >= 0.5 {
			continue
		}
		preFails++

		per := perSparkline[k]
		if per == nil || per.TotalRuns == 0 {
			continue
		}
		perPassRate := float64(per.ResultCount["S"]) / float64(per.TotalRuns)
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

func buildPeriodicHealth(id, name, prow, release, label, relationshipBasis, relationshipDescription string, periodicMap map[string]*SippyJob, sparklines map[string]map[string]*SparklineSlot) PeriodicJobHealth {
	d := periodicMap[prow]
	sparkline := sparklines[prow]
	counts := countResultTypes(sparkline)
	health := PeriodicJobHealth{
		ID:                      id,
		Name:                    name,
		Prow:                    prow,
		Release:                 release,
		Label:                   label,
		RelationshipBasis:       relationshipBasis,
		RelationshipDescription: relationshipDescription,
		TestFails:               counts.testFails,
		InfraFails:              counts.infraFails,
		SparkRuns:               counts.sparkRuns,
		Sparkline:               sparkline,
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

func transformWindow(raw *rawData, windowKey string, now time.Time, catalog *jobs.Catalog) *WindowData {
	win := WindowConfigs[windowKey]

	summaries := make(map[string]*SippyJob, len(raw.analyses))
	sparklines := make(map[string]map[string]*SparklineSlot, len(raw.analyses))
	for name, analysis := range raw.analyses {
		summaries[name] = summarizeAnalysis(analysis, windowKey, now)
		sparklines[name] = analysisSparkline(analysis, win, now)
	}

	blockingProwNames := make(map[string]bool)
	for _, bj := range catalog.BlockingJobs {
		blockingProwNames[bj.ProwJobName] = true
	}

	var jobHealths []JobHealth
	for _, cfg := range catalog.BlockingJobs {
		d := summaries[cfg.ProwJobName]

		var periodics []PeriodicJobHealth
		for _, cfgPer := range cfg.Periodics {
			periodics = append(periodics, buildPeriodicHealth(
				cfgPer.ID, cfgPer.Name, cfgPer.ProwJobName, cfgPer.Release, cfgPer.Release,
				string(cfgPer.RelationshipBasis), cfgPer.RelationshipDescription,
				summaries, sparklines,
			))
		}

		var correlation *Correlation
		if len(cfg.Periodics) > 0 {
			correlation = computeCorrelation(
				sparklines[cfg.ProwJobName],
				sparklines[cfg.Periodics[0].ProwJobName],
				now, win,
			)
		}

		preSparkline := sparklines[cfg.ProwJobName]
		preCounts := countResultTypes(preSparkline)

		release := ""
		if len(cfg.Periodics) > 0 {
			release = cfg.Periodics[0].Release
		}

		jh := JobHealth{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Prow:        cfg.ProwJobName,
			Platforms:   cfg.Platforms,
			Role:        string(cfg.Role),
			RoleLabel:   jobs.RoleLabel(cfg.Role, release),
			TestFails:   preCounts.testFails,
			InfraFails:  preCounts.infraFails,
			SparkRuns:   preCounts.sparkRuns,
			Periodics:   periodics,
			Sparkline:   preSparkline,
			Correlation: correlation,
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

	payloadHealths := make([]PayloadBlockingJobHealth, 0, len(catalog.PayloadBlockingJobs))
	for _, cfg := range catalog.PayloadBlockingJobs {
		health := buildPeriodicHealth(
			cfg.ID, cfg.Name, cfg.ProwJobName, cfg.Release, "release payload",
			"", "",
			summaries, sparklines,
		)
		participations := make([]ReleasePayloadParticipation, 0, len(cfg.Participations))
		for _, participation := range cfg.Participations {
			participations = append(participations, ReleasePayloadParticipation{
				StreamName:       participation.Stream.Name,
				StreamKind:       participation.Stream.Kind,
				Architecture:     participation.Stream.Architecture,
				VerificationName: participation.Verification.Name,
				StreamSippyURL:   participation.Stream.SippyURL,
				ReleaseStatusURL: participation.Stream.ReleaseStatusURL,
			})
		}
		payloadHealths = append(payloadHealths, PayloadBlockingJobHealth{
			PeriodicJobHealth: health,
			Platforms:         cfg.Platforms,
			Participations:    participations,
		})
	}

	componentReadinessHealths := make([]ComponentReadinessJobHealth, 0, len(raw.componentReadiness))
	for _, cfg := range raw.componentReadiness {
		health := buildPeriodicHealth(
			cfg.ID, cfg.Name, cfg.ProwJobName, cfg.Release, "component readiness",
			"", "",
			summaries, sparklines,
		)
		componentReadinessHealths = append(componentReadinessHealths, ComponentReadinessJobHealth{
			PeriodicJobHealth: health,
			Platforms:         cfg.Platforms,
			RegistryMissing:   cfg.RegistryMissing,
		})
	}

	alerts := buildAlerts(raw.recentFailures, blockingProwNames)

	return &WindowData{
		Jobs:                   jobHealths,
		PayloadBlockingJobs:    payloadHealths,
		ComponentReadinessJobs: componentReadinessHealths,
		Alerts:                 alerts,
	}
}

func buildAlerts(failures []SippyTestFailure, blockingProwNames map[string]bool) []Alert {
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
