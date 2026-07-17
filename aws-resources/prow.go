package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const prowDeckURL = "https://prow.ci.openshift.org"

func CheckProwJobs(ctx context.Context, httpClient *http.Client, graph *ResourceGraph) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, j := range graph.Jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j *JobNode) {
			defer wg.Done()
			defer func() { <-sem }()
			checkJob(ctx, httpClient, j)
		}(j)
	}

	wg.Wait()
}

var terminalStates = map[string]bool{
	"success": true,
	"failure": true,
	"aborted": true,
	"error":   true,
}

func checkJob(ctx context.Context, httpClient *http.Client, job *JobNode) {
	prowURL := fmt.Sprintf("%s/prowjob?prowjob=%s", prowDeckURL, job.ID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prowURL, nil)
	if err != nil {
		job.State = JobUnknown
		return
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  WARN: failed to fetch prowjob %s: %v\n", job.ID, err)
		job.State = JobUnknown
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		job.State = JobTerminal
		job.Result = "GC'd"
		return
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "  WARN: prowjob %s returned HTTP %d\n", job.ID, resp.StatusCode)
		job.State = JobUnknown
		return
	}

	var state, url, startTimeStr, completionTimeStr string
	inStatus := false
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "status:" {
			inStatus = true
			continue
		}
		if inStatus {
			if len(line) > 0 && line[0] != ' ' {
				break
			}
			trimmed := strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(trimmed, "state: "); ok {
				state = after
			} else if after, ok := strings.CutPrefix(trimmed, "url: "); ok {
				url = after
			} else if after, ok := strings.CutPrefix(trimmed, "startTime: "); ok {
				startTimeStr = strings.Trim(after, "\"")
			} else if after, ok := strings.CutPrefix(trimmed, "completionTime: "); ok {
				completionTimeStr = strings.Trim(after, "\"")
			}
		}
	}

	if state == "" {
		job.State = JobUnknown
		return
	}

	if terminalStates[state] {
		job.State = JobTerminal
		job.Result = strings.ToUpper(state)
	} else {
		job.State = JobRunning
	}

	if url != "" {
		job.ProwLink = url
	} else {
		job.ProwLink = prowURL
	}

	if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
		job.StartTime = &t
	}
	if t, err := time.Parse(time.RFC3339, completionTimeStr); err == nil {
		job.CompletionTime = &t
	}
}
