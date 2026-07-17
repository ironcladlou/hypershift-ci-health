package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func PrintTable(w io.Writer, graph *ResourceGraph) {
	fmt.Fprintf(w, "%-10s %-10s %5s  %-60s %s\n", "STATE", "RESULT", "COUNT", "RESOURCE TYPES", "PROW JOB ID")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 160))

	for _, job := range graph.Jobs {
		result := job.Result
		if result == "" {
			result = "-"
		}
		types := formatResourceTypes(job)
		fmt.Fprintf(w, "%-10s %-10s %5d  %-60s %s\n",
			job.State, result, len(job.Resources), types, job.ID)
		if job.ProwLink != "" {
			fmt.Fprintf(w, "%28s%s\n", "", job.ProwLink)
		}
	}
}

func PrintSummary(w io.Writer, graph *ResourceGraph) {
	s := graph.Summary()
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "==============================\n")
	fmt.Fprintf(w, "  Summary\n")
	fmt.Fprintf(w, "==============================\n")
	fmt.Fprintf(w, "  Terminal (orphaned): %d jobs (%d resources)\n", s.TerminalJobs, s.TerminalResources)
	fmt.Fprintf(w, "  Running:            %d jobs (%d resources)\n", s.RunningJobs, s.RunningResources)
	if s.UnknownJobs > 0 {
		fmt.Fprintf(w, "  Unknown (stale):    %d jobs (%d resources)\n", s.UnknownJobs, s.UnknownResources)
	}
	fmt.Fprintf(w, "  Total:              %d jobs (%d resources)\n", s.TotalJobs, s.TotalResources)
	fmt.Fprintf(w, "==============================\n")
}

func PrintJSON(w io.Writer, graph *ResourceGraph) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(graph)
}

func formatResourceTypes(job *JobNode) string {
	if len(job.Resources) == 0 {
		return "(none)"
	}
	counts := make(map[string]int)
	for _, r := range job.Resources {
		counts[r.Type]++
	}
	parts := make([]string, 0, len(counts))
	for resType, count := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", resType, count))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
