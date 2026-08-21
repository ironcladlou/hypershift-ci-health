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
}

var WindowConfigs = map[string]WindowConfig{
	"2d": {SparkSlotHours: 2, SparkSlots: 24},
	"7d": {SparkSlotHours: 6, SparkSlots: 28},
}

type rawData struct {
	presubmitJobs    map[string][]SippyJob
	periodicJobs     map[string][]SippyJob
	presubmitRuns    []SippyJobRun
	periodicRuns     []SippyJobRun
	recentFailures   []SippyTestFailure
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

// bucketJobRuns groups job runs into time-slot sparklines.
// Returns map[jobName]map[slotKey]*SparklineSlot.
func bucketJobRuns(runs []SippyJobRun, win WindowConfig, now time.Time) map[string]map[string]*SparklineSlot {
	cutoff := now.Add(-time.Duration(win.SparkSlots*win.SparkSlotHours) * time.Hour)
	cutoffMS := cutoff.UnixMilli()

	data := make(map[string]map[string]*SparklineSlot)
	for _, run := range runs {
		if run.Timestamp < cutoffMS {
			continue
		}
		t := time.UnixMilli(run.Timestamp).UTC()
		key := slotKey(t, win.SparkSlotHours)

		jobData, ok := data[run.Job]
		if !ok {
			jobData = make(map[string]*SparklineSlot)
			data[run.Job] = jobData
		}
		slot, ok := jobData[key]
		if !ok {
			slot = &SparklineSlot{ResultCount: make(map[string]int)}
			jobData[key] = slot
		}
		slot.TotalRuns++
		rc := run.OverallResult
		if rc == "" {
			if run.Succeeded {
				rc = "S"
			} else {
				rc = "F"
			}
		}
		slot.ResultCount[rc]++
	}
	return data
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

type flakeStats struct {
	runs   int
	flakes int
}

func computeFlakes(runs []SippyJobRun, cutoffMS int64) map[string]*flakeStats {
	flakes := make(map[string]*flakeStats)
	for _, run := range runs {
		if run.Timestamp < cutoffMS {
			continue
		}
		fs, ok := flakes[run.Job]
		if !ok {
			fs = &flakeStats{}
			flakes[run.Job] = fs
		}
		fs.runs++
		if run.TestFlakes > 0 {
			fs.flakes++
		}
	}
	return flakes
}

func transformWindow(raw *rawData, windowKey string, now time.Time) *WindowData {
	win := WindowConfigs[windowKey]

	presubmitMap := make(map[string]*SippyJob)
	if jobList, ok := raw.presubmitJobs[windowKey]; ok {
		for i := range jobList {
			presubmitMap[jobList[i].Name] = &jobList[i]
		}
	}

	periodicMap := make(map[string]*SippyJob)
	if jobList, ok := raw.periodicJobs[windowKey]; ok {
		for i := range jobList {
			periodicMap[jobList[i].Name] = &jobList[i]
		}
	}

	sparklines := bucketJobRuns(raw.presubmitRuns, win, now)
	periodicSparklines := bucketJobRuns(raw.periodicRuns, win, now)

	cutoffMS := now.Add(-time.Duration(win.SparkSlots*win.SparkSlotHours) * time.Hour).UnixMilli()
	periodicFlakes := computeFlakes(raw.periodicRuns, cutoffMS)

	blockingProwNames := make(map[string]bool)
	for _, bj := range jobs.BlockingJobs {
		blockingProwNames[bj.ProwJobName] = true
	}

	var jobHealths []JobHealth
	for _, cfg := range jobs.BlockingJobs {
		d := presubmitMap[cfg.ProwJobName]

		var periodics []PeriodicJobHealth
		for _, cfgPer := range cfg.Periodics {
			pd := periodicMap[cfgPer.ProwJobName]
			perSparkline := periodicSparklines[cfgPer.ProwJobName]
			perCounts := countResultTypes(perSparkline)

			pjh := PeriodicJobHealth{
				Name:       cfgPer.Name,
				Prow:       cfgPer.ProwJobName,
				Release:    cfgPer.Release,
				Label:      cfgPer.Release,
				TestFails:  perCounts.testFails,
				InfraFails: perCounts.infraFails,
				SparkRuns:  perCounts.sparkRuns,
				Sparkline:  perSparkline,
			}

			if pd != nil {
				pjh.Rate = &pd.CurrentPassPercentage
				pjh.Prev = &pd.PreviousPassPercentage
				pjh.PrevRuns = pd.PreviousRuns
				pjh.Trend = &pd.NetImprovement
				pjh.Runs = pd.CurrentRuns
				pjh.Fails = pd.CurrentFails
			}

			if fs := periodicFlakes[cfgPer.ProwJobName]; fs != nil {
				pjh.FlakyRuns = fs.flakes
				pjh.TotalRuns = fs.runs
			}

			periodics = append(periodics, pjh)
		}

		var correlation *Correlation
		if len(cfg.Periodics) > 0 {
			correlation = computeCorrelation(
				sparklines[cfg.ProwJobName],
				periodicSparklines[cfg.Periodics[0].ProwJobName],
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
			Name:        cfg.Name,
			Prow:        cfg.ProwJobName,
			Platform:    string(cfg.Platform),
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

	alerts := buildAlerts(raw.recentFailures, blockingProwNames)

	return &WindowData{
		Jobs:   jobHealths,
		Alerts: alerts,
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
