package collector

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDeletionPlan_OnlyTerminalJobs(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "terminal-job", State: JobTerminal, Result: "FAILURE", CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-1"},
			}},
			{ID: "running-job", State: JobRunning, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-2"},
			}},
			{ID: "unknown-job", State: JobUnknown, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-3"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)

	if plan.Summary.Jobs != 1 {
		t.Fatalf("expected 1 job, got %d", plan.Summary.Jobs)
	}
	if plan.Jobs[0].JobID != "terminal-job" {
		t.Fatalf("expected terminal-job, got %s", plan.Jobs[0].JobID)
	}
}

func TestBuildDeletionPlan_MinAgeFilter(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour)
	old := now.Add(-6 * time.Hour)

	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "recent", State: JobTerminal, CompletionTime: &recent, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-1"},
			}},
			{ID: "old", State: JobTerminal, CompletionTime: &old, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-2"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 4*time.Hour)

	if plan.Summary.Jobs != 1 {
		t.Fatalf("expected 1 job, got %d", plan.Summary.Jobs)
	}
	if plan.Jobs[0].JobID != "old" {
		t.Fatalf("expected old job, got %s", plan.Jobs[0].JobID)
	}
	if plan.MinAge != "4h0m0s" {
		t.Fatalf("expected minAge 4h0m0s, got %s", plan.MinAge)
	}
}

func TestBuildDeletionPlan_StepOrdering(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-1"},
				{Type: "ec2:instance", Region: "us-east-1", ID: "i-1"},
				{Type: "ec2:subnet", Region: "us-east-1", ID: "subnet-1"},
				{Type: "ec2:natgateway", Region: "us-east-1", ID: "nat-1"},
				{Type: "iam:role", ID: "my-role", ARN: "arn:aws:iam::123:role/my-role"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)
	steps := plan.Jobs[0].Steps

	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	for i := 1; i < len(steps); i++ {
		if steps[i].Order < steps[i-1].Order {
			t.Errorf("step %d (order %d, %s) comes after step %d (order %d, %s) but has lower order",
				i, steps[i].Order, steps[i].Type, i-1, steps[i-1].Order, steps[i-1].Type)
		}
	}

	if steps[0].Type != "ec2:instance" {
		t.Errorf("expected instance first, got %s", steps[0].Type)
	}
	if steps[len(steps)-1].Type != "iam:role" {
		t.Errorf("expected IAM role last, got %s", steps[len(steps)-1].Type)
	}
}

func TestBuildDeletionPlan_PlanIDDeterminism(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)

	makeData := func(order ...string) *APIResponse {
		var resources []*ResourceNode
		for _, id := range order {
			resources = append(resources, &ResourceNode{Type: "ec2:vpc", Region: "us-east-1", ID: id})
		}
		return &APIResponse{
			Jobs: []*JobNode{{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: resources}},
		}
	}

	plan1 := BuildDeletionPlan(makeData("vpc-a", "vpc-b", "vpc-c"), 0)
	plan2 := BuildDeletionPlan(makeData("vpc-c", "vpc-a", "vpc-b"), 0)

	if plan1.PlanID != plan2.PlanID {
		t.Fatalf("plan IDs differ for same resources in different order: %s vs %s", plan1.PlanID, plan2.PlanID)
	}
}

func TestBuildDeletionPlan_EmptyInput(t *testing.T) {
	plan := BuildDeletionPlan(&APIResponse{}, 0)

	if plan.Summary.Jobs != 0 || plan.Summary.Resources != 0 {
		t.Fatalf("expected empty plan, got %d jobs %d resources", plan.Summary.Jobs, plan.Summary.Resources)
	}
	if len(plan.PlanID) == 0 {
		t.Fatal("expected non-empty plan ID even for empty plan")
	}
}

func TestBuildDeletionPlan_UnknownResourceTypeSkipped(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-1"},
				{Type: "unknown:thing", Region: "us-east-1", ID: "x-1"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)

	if plan.Summary.Resources != 1 {
		t.Fatalf("expected 1 resource (unknown skipped), got %d", plan.Summary.Resources)
	}
}

func TestBuildDeletionPlan_CLICommands(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:instance", Region: "us-east-1", ID: "i-abc123"},
				{Type: "iam:role", ID: "my-role", ARN: "arn:aws:iam::123:role/my-role"},
				{Type: "route53:hostedzone", ID: "Z1234", ARN: "arn:aws:route53:::hostedzone/Z1234"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)

	expected := map[string]string{
		"ec2:instance":       "aws ec2 terminate-instances --instance-ids i-abc123 --region us-east-1",
		"iam:role":           "aws iam delete-role --role-name my-role",
		"route53:hostedzone": "aws route53 delete-hosted-zone --id Z1234",
	}

	for _, step := range plan.Jobs[0].Steps {
		want, ok := expected[step.Type]
		if !ok {
			continue
		}
		if step.CLICommand != want {
			t.Errorf("type %s: got %q, want %q", step.Type, step.CLICommand, want)
		}
	}
}

func TestBuildDeletionPlan_ConsoleURL(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-1", ConsoleURL: "https://console.aws.amazon.com/vpc/home?region=us-east-1#VpcDetails:VpcId=vpc-1"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)

	if len(plan.Jobs) != 1 || len(plan.Jobs[0].Steps) != 1 {
		t.Fatal("expected 1 job with 1 step")
	}
	step := plan.Jobs[0].Steps[0]
	if step.ConsoleURL == "" {
		t.Fatal("expected ConsoleURL to be populated")
	}
	if step.ConsoleURL != "https://console.aws.amazon.com/vpc/home?region=us-east-1#VpcDetails:VpcId=vpc-1" {
		t.Fatalf("unexpected ConsoleURL: %s", step.ConsoleURL)
	}
}

func TestRenderScript_Structure(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-abc", State: JobTerminal, Result: "FAILURE", CompletionTime: &past, ProwLink: "https://prow.ci.openshift.org/view/gs/test/job-abc", Resources: []*ResourceNode{
				{Type: "ec2:instance", Region: "us-east-1", ID: "i-111"},
				{Type: "ec2:vpc", Region: "us-east-1", ID: "vpc-222"},
			}},
			{ID: "job-def", State: JobTerminal, Result: "SUCCESS", CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:subnet", Region: "us-west-2", ID: "subnet-333", Name: "my-subnet"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)
	script := RenderScript(plan)

	checks := []struct {
		desc    string
		content string
	}{
		{"shebang", "#!/usr/bin/env bash"},
		{"plan ID", plan.PlanID[:16]},
		{"confirmation prompt", `read -r -p "Proceed with cleanup? [y/N] " confirm`},
		{"job-abc header", "# Job: job-abc"},
		{"job-def header", "# Job: job-def"},
		{"instance CLI", "aws ec2 terminate-instances --instance-ids i-111 --region us-east-1"},
		{"vpc CLI", "aws ec2 delete-vpc --vpc-id vpc-222 --region us-east-1"},
		{"subnet CLI", "aws ec2 delete-subnet --subnet-id subnet-333 --region us-west-2"},
		{"step numbering", "[1/3]"},
		{"last step numbering", "[3/3]"},
		{"summary section", "Cleanup complete"},
		{"failure counter", "$FAILED"},
	}
	for _, c := range checks {
		if !strings.Contains(script, c.content) {
			t.Errorf("script missing %s: expected to contain %q", c.desc, c.content)
		}
	}
}

func TestRenderScript_Empty(t *testing.T) {
	plan := BuildDeletionPlan(&APIResponse{}, 0)
	script := RenderScript(plan)

	if !strings.HasPrefix(script, "#!/usr/bin/env bash") {
		t.Fatal("expected script to start with shebang")
	}
	if !strings.Contains(script, "Cleanup complete") {
		t.Fatal("expected summary section in empty script")
	}
	if strings.Contains(script, "# Job:") {
		t.Fatal("expected no job sections in empty script")
	}
}

func TestRenderScript_Notes(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-6 * time.Hour)
	data := &APIResponse{
		Jobs: []*JobNode{
			{ID: "job-1", State: JobTerminal, CompletionTime: &past, Resources: []*ResourceNode{
				{Type: "ec2:internet-gateway", Region: "us-east-1", ID: "igw-1"},
			}},
		},
	}

	plan := BuildDeletionPlan(data, 0)
	script := RenderScript(plan)

	if !strings.Contains(script, "# Note: Must be detached from VPC first") {
		t.Fatal("expected note for internet gateway in script")
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		offset time.Duration
		want   string
	}{
		{26 * time.Hour, "1d 2h"},
		{3 * time.Hour, "3h 0m"},
		{90 * time.Minute, "1h 30m"},
		{15 * time.Minute, "15m"},
	}
	for _, tt := range tests {
		got := formatAge(now, now.Add(-tt.offset))
		if got != tt.want {
			t.Errorf("formatAge(-%v) = %q, want %q", tt.offset, got, tt.want)
		}
	}
}
