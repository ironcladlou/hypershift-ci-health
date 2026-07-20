package retests

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var logWriter io.Writer = os.Stderr

type Config struct {
	Org         string
	Repo        string
	GitHubToken string
	WindowDays  int
	Concurrency int
}

func Run(ctx context.Context, cfg Config) (*AnalysisResult, error) {
	since := time.Now().UTC().AddDate(0, 0, -cfg.WindowDays)

	fmt.Fprintf(logWriter, "Fetching merged PRs since %s...\n", since.Format("2006-01-02"))
	prs, err := FetchMergedPRs(ctx, cfg.GitHubToken, cfg.Org, cfg.Repo, since)
	if err != nil {
		return nil, fmt.Errorf("fetching merged PRs: %w", err)
	}
	fmt.Fprintf(logWriter, "Found %d merged PRs\n", len(prs))

	if len(prs) == 0 {
		return &AnalysisResult{
			GeneratedAt: time.Now().UTC(),
			WindowDays:  cfg.WindowDays,
			Org:         cfg.Org,
			Repo:        cfg.Repo,
		}, nil
	}

	blockingSet := make(map[string]string)
	for _, bj := range BlockingJobs {
		blockingSet[bj.ProwJobName] = bj.Name
	}

	type scrapeResult struct {
		pr      MergedPR
		history *ProwPRHistory
		err     error
	}

	results := make([]scrapeResult, len(prs))
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for i, pr := range prs {
		wg.Add(1)
		go func(i int, pr MergedPR) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			history, err := FetchPRHistory(ctx, cfg.Org, cfg.Repo, pr.Number)
			results[i] = scrapeResult{pr: pr, history: history, err: err}
			if err != nil {
				fmt.Fprintf(logWriter, "  PR #%d: error: %v\n", pr.Number, err)
			} else {
				fmt.Fprintf(logWriter, "  PR #%d: %d jobs scraped\n", pr.Number, len(history.Jobs))
			}
		}(i, pr)
	}
	wg.Wait()

	var prResults []PRResult
	for _, r := range results {
		if r.err != nil {
			continue
		}
		pr := analyzePR(r.pr, r.history, blockingSet)
		prResults = append(prResults, pr)
	}

	summary := computeSummary(prResults, blockingSet)

	return &AnalysisResult{
		GeneratedAt: time.Now().UTC(),
		WindowDays:  cfg.WindowDays,
		Org:         cfg.Org,
		Repo:        cfg.Repo,
		PRsAnalyzed: len(prResults),
		PRs:         prResults,
		Summary:     summary,
	}, nil
}

func analyzePR(pr MergedPR, history *ProwPRHistory, blockingSet map[string]string) PRResult {
	shortSHA := pr.HeadSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	result := PRResult{
		Number:   pr.Number,
		Title:    pr.Title,
		Author:   pr.Author,
		MergedAt: pr.MergedAt,
		HeadSHA:  pr.HeadSHA,
		Jobs:     []PRJobResult{},
	}

	for _, job := range history.Jobs {
		displayName, isBlocking := blockingSet[job.Name]
		if !isBlocking {
			continue
		}

		var runs, failures, aborts int
		for _, run := range job.Runs {
			if !strings.HasPrefix(run.CommitSHA, shortSHA) && !strings.HasPrefix(shortSHA, run.CommitSHA) {
				continue
			}
			runs++
			switch run.Status {
			case RunFailure:
				failures++
			case RunAborted:
				aborts++
			}
		}

		if runs == 0 {
			continue
		}

		retests := runs - 1
		result.Jobs = append(result.Jobs, PRJobResult{
			Name:     displayName,
			Runs:     runs,
			Failures: failures,
			Aborts:   aborts,
			Retests:  retests,
		})

		if retests > result.MaxRetest {
			result.MaxRetest = retests
		}
	}

	return result
}

func computeSummary(prs []PRResult, blockingSet map[string]string) Summary {
	summary := Summary{}

	if len(prs) == 0 {
		return summary
	}

	var retestCounts []int
	jobStats := make(map[string]*JobSummary)

	for _, pr := range prs {
		if len(pr.Jobs) == 0 {
			continue
		}
		retestCounts = append(retestCounts, pr.MaxRetest)
		summary.TotalRetestRounds += pr.MaxRetest

		for _, job := range pr.Jobs {
			js, ok := jobStats[job.Name]
			if !ok {
				prowName := ""
				for _, bj := range BlockingJobs {
					if bj.Name == job.Name {
						prowName = bj.ProwJobName
						break
					}
				}
				js = &JobSummary{Name: prowName, DisplayName: job.Name}
				jobStats[job.Name] = js
			}
			js.TotalRuns += job.Runs
			js.TotalFailures += job.Failures
			if job.Retests > 0 {
				js.PRsAffected++
			}
		}
	}

	sort.Ints(retestCounts)
	summary.MedianRetests = percentile(retestCounts, 50)
	summary.P90Retests = percentile(retestCounts, 90)
	summary.P95Retests = percentile(retestCounts, 95)

	var perJob []JobSummary
	mergeProbability := 1.0
	for _, bj := range BlockingJobs {
		js, ok := jobStats[bj.Name]
		if !ok {
			continue
		}
		if js.TotalRuns > 0 {
			js.PassRate = float64(js.TotalRuns-js.TotalFailures) / float64(js.TotalRuns)
		}

		var jobRetests []int
		for _, pr := range prs {
			for _, job := range pr.Jobs {
				if job.Name == bj.Name {
					jobRetests = append(jobRetests, job.Retests)
				}
			}
		}
		if len(jobRetests) > 0 {
			sort.Ints(jobRetests)
			js.MedianRetests = percentile(jobRetests, 50)
		}

		perJob = append(perJob, *js)

		if js.PassRate == 0 {
			summary.QueueBlockers = append(summary.QueueBlockers, bj.Name)
		}
		mergeProbability *= js.PassRate
	}

	summary.PerJob = perJob
	summary.FirstTryMergeProbability = mergeProbability

	return summary
}

func percentile(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
