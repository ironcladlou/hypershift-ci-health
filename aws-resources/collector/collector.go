package collector

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
)

type JobState string

const (
	JobRunning  JobState = "RUNNING"
	JobTerminal JobState = "TERMINAL"
	JobUnknown  JobState = "UNKNOWN"
)

type ResourceGraph struct {
	Jobs []*JobNode `json:"jobs"`
}

type JobNode struct {
	ID             string          `json:"id"`
	State          JobState        `json:"state"`
	Result         string          `json:"result,omitempty"`
	ProwLink       string          `json:"prowLink,omitempty"`
	StartTime      *time.Time      `json:"startTime,omitempty"`
	CompletionTime *time.Time      `json:"completionTime,omitempty"`
	Resources      []*ResourceNode `json:"resources"`
}

type ResourceNode struct {
	ARN         string `json:"arn"`
	Type        string `json:"type"`
	Region      string `json:"region"`
	ID          string `json:"id"`
	ConsoleURL  string `json:"consoleURL,omitempty"`
	InfraID     string `json:"infraID,omitempty"`
	ClusterName string `json:"clusterName,omitempty"`
	Name        string `json:"name,omitempty"`
}

type GraphSummary struct {
	TerminalJobs      int `json:"terminalJobs"`
	TerminalResources int `json:"terminalResources"`
	RunningJobs       int `json:"runningJobs"`
	RunningResources  int `json:"runningResources"`
	UnknownJobs       int `json:"unknownJobs"`
	UnknownResources  int `json:"unknownResources"`
	TotalJobs         int `json:"totalJobs"`
	TotalResources    int `json:"totalResources"`
}

type APIResponse struct {
	GeneratedAt string       `json:"generatedAt"`
	Summary     GraphSummary `json:"summary"`
	Jobs        []*JobNode   `json:"jobs"`
}

type Config struct {
	Regions []string
	JobID   string
}

var defaultRegions = []string{
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
}

func DefaultConfig() Config {
	return Config{
		Regions: defaultRegions,
	}
}

func (g *ResourceGraph) Orphans() []*JobNode {
	var out []*JobNode
	for _, j := range g.Jobs {
		if j.State == JobTerminal {
			out = append(out, j)
		}
	}
	return out
}

func (g *ResourceGraph) Running() []*JobNode {
	var out []*JobNode
	for _, j := range g.Jobs {
		if j.State == JobRunning {
			out = append(out, j)
		}
	}
	return out
}

func (g *ResourceGraph) Summary() GraphSummary {
	var s GraphSummary
	for _, j := range g.Jobs {
		n := len(j.Resources)
		switch j.State {
		case JobTerminal:
			s.TerminalJobs++
			s.TerminalResources += n
		case JobRunning:
			s.RunningJobs++
			s.RunningResources += n
		default:
			s.UnknownJobs++
			s.UnknownResources += n
		}
	}
	s.TotalJobs = len(g.Jobs)
	s.TotalResources = s.TerminalResources + s.RunningResources + s.UnknownResources
	return s
}

func (g *ResourceGraph) Merge(other *ResourceGraph) {
	index := make(map[string]*JobNode, len(g.Jobs))
	for _, j := range g.Jobs {
		index[j.ID] = j
	}
	for _, j := range other.Jobs {
		if existing, ok := index[j.ID]; ok {
			existing.Resources = append(existing.Resources, j.Resources...)
		} else {
			g.Jobs = append(g.Jobs, j)
			index[j.ID] = j
		}
	}
}

func (g *ResourceGraph) Sort() {
	stateOrder := map[JobState]int{JobTerminal: 0, JobUnknown: 1, JobRunning: 2}
	sort.Slice(g.Jobs, func(i, j int) bool {
		if g.Jobs[i].State != g.Jobs[j].State {
			return stateOrder[g.Jobs[i].State] < stateOrder[g.Jobs[j].State]
		}
		ni, nj := len(g.Jobs[i].Resources), len(g.Jobs[j].Resources)
		if ni != nj {
			return ni > nj
		}
		return g.Jobs[i].ID < g.Jobs[j].ID
	})
}

func Collect(ctx context.Context, cfg Config) (*APIResponse, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	graph := &ResourceGraph{}
	for _, region := range cfg.Regions {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("loading AWS config for %s: %w", region, err)
		}

		taggingClient := resourcegroupstaggingapi.NewFromConfig(awsCfg)

		fmt.Fprintf(os.Stderr, "Discovering tagged resources in %s...\n", region)
		regionGraph, err := Discover(ctx, taggingClient, cfg.JobID)
		if err != nil {
			return nil, fmt.Errorf("discovery failed in %s: %w", region, err)
		}

		graph.Merge(regionGraph)
	}

	if len(graph.Jobs) > 0 {
		fmt.Fprintf(os.Stderr, "Checking %d prow job(s) for terminal state...\n", len(graph.Jobs))
		CheckProwJobs(ctx, httpClient, graph)
		graph.Sort()
	}

	return &APIResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     graph.Summary(),
		Jobs:        graph.Jobs,
	}, nil
}
