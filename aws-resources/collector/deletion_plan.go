package collector

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

type DeletionPlan struct {
	PlanID      string         `json:"planID"`
	GeneratedAt string         `json:"generatedAt"`
	MinAge      string         `json:"minAge,omitempty"`
	Summary     PlanSummary    `json:"summary"`
	Jobs        []*JobDeletion `json:"jobs"`
}

type PlanSummary struct {
	Jobs      int `json:"jobs"`
	Resources int `json:"resources"`
}

type JobDeletion struct {
	JobID          string          `json:"jobID"`
	Result         string          `json:"result,omitempty"`
	ProwLink       string          `json:"prowLink,omitempty"`
	CompletionTime *time.Time      `json:"completionTime,omitempty"`
	Age            string          `json:"age,omitempty"`
	Steps          []*DeletionStep `json:"steps"`
}

type DeletionStep struct {
	Order       int    `json:"order"`
	ResourceID  string `json:"resourceID"`
	ResourceARN string `json:"resourceARN,omitempty"`
	Type        string `json:"type"`
	Region      string `json:"region,omitempty"`
	Name        string `json:"name,omitempty"`
	ConsoleURL  string `json:"consoleURL,omitempty"`
	Action      string `json:"action"`
	CLICommand  string `json:"cliCommand"`
	Note        string `json:"note,omitempty"`
}

type deleteAction struct {
	order  int
	action string
	cliCmd func(res *ResourceNode) string
	note   string
}

func ec2Cmd(verb, flag string) func(*ResourceNode) string {
	return func(res *ResourceNode) string {
		return fmt.Sprintf("aws ec2 %s --%s %s --region %s", verb, flag, res.ID, res.Region)
	}
}

func iamCmd(verb, flag string) func(*ResourceNode) string {
	return func(res *ResourceNode) string {
		return fmt.Sprintf("aws iam %s --%s %s", verb, flag, res.ID)
	}
}

var deleteActions = map[string]deleteAction{
	"ec2:instance":             {1, "Terminate Instance", ec2Cmd("terminate-instances", "instance-ids"), ""},
	"ec2:capacity-reservation": {1, "Cancel Capacity Reservation", ec2Cmd("cancel-capacity-reservation", "capacity-reservation-id"), ""},
	"ec2:key-pair":             {1, "Delete Key Pair", ec2Cmd("delete-key-pair", "key-pair-id"), ""},
	"ec2:natgateway":           {2, "Delete NAT Gateway", ec2Cmd("delete-nat-gateway", "nat-gateway-id"), "May take several minutes to complete"},
	"ec2:vpc-endpoint":         {2, "Delete VPC Endpoint", ec2Cmd("delete-vpc-endpoints", "vpc-endpoint-ids"), ""},
	"ec2:elastic-ip":           {3, "Release Elastic IP", ec2Cmd("release-address", "allocation-id"), "Will fail if still associated; NAT gateway deletion releases the association"},
	"ec2:security-group":       {3, "Delete Security Group", ec2Cmd("delete-security-group", "group-id"), ""},
	"ec2:internet-gateway":     {3, "Delete Internet Gateway", ec2Cmd("delete-internet-gateway", "internet-gateway-id"), "Must be detached from VPC first"},
	"ec2:subnet":               {4, "Delete Subnet", ec2Cmd("delete-subnet", "subnet-id"), ""},
	"ec2:route-table":          {4, "Delete Route Table", ec2Cmd("delete-route-table", "route-table-id"), "Main route table cannot be deleted directly; it is removed with the VPC"},
	"ec2:vpc":                  {5, "Delete VPC", ec2Cmd("delete-vpc", "vpc-id"), ""},
	"ec2:dhcp-options":         {6, "Delete DHCP Options", ec2Cmd("delete-dhcp-options", "dhcp-options-id"), "Only deletable after the VPC using it is deleted"},
	"iam:instance-profile":     {7, "Delete Instance Profile", iamCmd("delete-instance-profile", "instance-profile-name"), "Remove attached roles first"},
	"iam:role":                 {8, "Delete IAM Role", iamCmd("delete-role", "role-name"), "Detach all policies and remove from instance profiles first"},
	"iam:oidc-provider": {9, "Delete OIDC Provider", func(res *ResourceNode) string {
		return fmt.Sprintf("aws iam delete-open-id-connect-provider --open-id-connect-provider-arn %s", res.ARN)
	}, ""},
	"route53:hostedzone": {10, "Delete Hosted Zone", func(res *ResourceNode) string {
		return fmt.Sprintf("aws route53 delete-hosted-zone --id %s", res.ID)
	}, "Delete all non-NS/non-SOA record sets first"},
}

func BuildDeletionPlan(data *APIResponse, minAge time.Duration) *DeletionPlan {
	now := time.Now().UTC()
	plan := &DeletionPlan{
		GeneratedAt: now.Format(time.RFC3339),
	}
	if minAge > 0 {
		plan.MinAge = minAge.String()
	}

	for _, job := range data.Jobs {
		if job.State != JobTerminal {
			continue
		}
		if minAge > 0 && job.CompletionTime != nil && now.Sub(*job.CompletionTime) < minAge {
			continue
		}

		jd := &JobDeletion{
			JobID:          job.ID,
			Result:         job.Result,
			ProwLink:       job.ProwLink,
			CompletionTime: job.CompletionTime,
		}
		if job.CompletionTime != nil {
			jd.Age = formatAge(now, *job.CompletionTime)
		}

		for _, res := range job.Resources {
			act, ok := deleteActions[res.Type]
			if !ok {
				continue
			}
			jd.Steps = append(jd.Steps, &DeletionStep{
				Order:       act.order,
				ResourceID:  res.ID,
				ResourceARN: res.ARN,
				Type:        res.Type,
				Region:      res.Region,
				Name:        res.Name,
				ConsoleURL:  res.ConsoleURL,
				Action:      act.action,
				CLICommand:  act.cliCmd(res),
				Note:        act.note,
			})
		}

		sort.Slice(jd.Steps, func(i, j int) bool {
			if jd.Steps[i].Order != jd.Steps[j].Order {
				return jd.Steps[i].Order < jd.Steps[j].Order
			}
			if jd.Steps[i].Type != jd.Steps[j].Type {
				return jd.Steps[i].Type < jd.Steps[j].Type
			}
			return jd.Steps[i].ResourceID < jd.Steps[j].ResourceID
		})

		plan.Jobs = append(plan.Jobs, jd)
	}

	plan.Summary.Jobs = len(plan.Jobs)
	for _, j := range plan.Jobs {
		plan.Summary.Resources += len(j.Steps)
	}
	plan.PlanID = computePlanID(plan.Jobs)

	return plan
}

func computePlanID(jobs []*JobDeletion) string {
	var keys []string
	for _, j := range jobs {
		for _, s := range j.Steps {
			keys = append(keys, s.Type+":"+s.Region+":"+s.ResourceID)
		}
	}
	sort.Strings(keys)
	h := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return fmt.Sprintf("%x", h)
}

type scriptData struct {
	ShortID    string
	Plan       *DeletionPlan
	TotalSteps int
	Jobs       []scriptJob
}

type scriptJob struct {
	*JobDeletion
	Steps []scriptStep
}

type scriptStep struct {
	*DeletionStep
	Num int
}

var scriptTmpl = template.Must(template.New("script").Parse(`#!/usr/bin/env bash
# ============================================================
# AWS Orphan Resource Cleanup Script
# Plan ID:    {{ .ShortID }}
# Generated:  {{ .Plan.GeneratedAt }}
# Min Age:    {{ or .Plan.MinAge "none" }}
# Jobs:       {{ .Plan.Summary.Jobs }}
# Resources:  {{ .Plan.Summary.Resources }}
# ============================================================
#
# Generated by the HyperShift E2E Resource Monitor.
# Review carefully before executing.
# ============================================================

set -uo pipefail

FAILED=0
SUCCEEDED=0

echo ""
echo "AWS Orphan Resource Cleanup"
echo "Plan ID:   {{ .ShortID }}"
echo "Scope:     {{ .Plan.Summary.Jobs }} jobs, {{ .Plan.Summary.Resources }} resources"
echo ""
read -r -p "Proceed with cleanup? [y/N] " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "Aborted."
  exit 0
fi
{{ range .Jobs }}
# ------------------------------------------------------------
# Job: {{ .JobID }}
# Result: {{ .Result }}  Age: {{ .Age }}
# Prow: {{ .ProwLink }}
# Resources: {{ len .Steps }}
# ------------------------------------------------------------
{{ range .Steps }}
# [{{ .Num }}/{{ $.TotalSteps }}] {{ .Action }} ({{ .Type }})
# Resource: {{ .ResourceID }}  Region: {{ .Region }}  Name: {{ .Name }}
{{- if .Note }}
# Note: {{ .Note }}
{{- end }}
echo "  [{{ .Num }}/{{ $.TotalSteps }}] {{ .Action }}: {{ .ResourceID }}..."
if {{ .CLICommand }}; then
  ((SUCCEEDED++))
else
  echo "  FAILED: {{ .CLICommand }}"
  ((FAILED++))
fi
{{ end }}{{ end }}
# ============================================================
# Summary
# ============================================================
echo ""
echo "============================================================"
echo "Cleanup complete"
echo "  Succeeded: $SUCCEEDED"
echo "  Failed:    $FAILED"
echo "============================================================"
if [[ $FAILED -gt 0 ]]; then
  echo "WARNING: $FAILED command(s) failed. Review output above."
  exit 1
fi
`))

func RenderScript(plan *DeletionPlan) string {
	shortID := plan.PlanID
	if len(shortID) > 16 {
		shortID = shortID[:16]
	}

	totalSteps := 0
	var jobs []scriptJob
	for _, j := range plan.Jobs {
		sj := scriptJob{JobDeletion: j}
		for _, s := range j.Steps {
			totalSteps++
			sj.Steps = append(sj.Steps, scriptStep{DeletionStep: s, Num: totalSteps})
		}
		jobs = append(jobs, sj)
	}

	var buf bytes.Buffer
	scriptTmpl.Execute(&buf, scriptData{
		ShortID:    shortID,
		Plan:       plan,
		TotalSteps: totalSteps,
		Jobs:       jobs,
	})
	return buf.String()
}

func formatAge(now, t time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
