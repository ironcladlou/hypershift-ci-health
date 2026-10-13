package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobpolicy"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/releaseconfig"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/reportplan"
	"go.yaml.in/yaml/v3"
)

func TestSharedDevelopmentDefaults(t *testing.T) {
	for _, name := range []string{"job-registry", "report-plan", "job-policy"} {
		command, _, err := NewRootCommand().Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		if command.Flags().Lookup("development-release").DefValue != releaseconfig.DevelopmentRelease || command.Flags().Lookup("development-branch").DefValue != releaseconfig.DevelopmentBranch {
			t.Fatalf("%s has divergent defaults", name)
		}
	}
	selection := reportplan.DefaultSelectionPolicy("", "")
	if selection.DevelopmentRelease != releaseconfig.DevelopmentRelease || selection.MaximumRelease != releaseconfig.DevelopmentRelease || selection.DevelopmentBranch != releaseconfig.DevelopmentBranch {
		t.Fatal("report plan defaults disagree")
	}
	registry, err := jobregistry.LoadFile("../jobregistry/testdata/job-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateDevelopmentRelease(releaseconfig.DevelopmentBranch, releaseconfig.DevelopmentRelease); err != nil {
		t.Fatal(err)
	}
	if _, err := reportplan.Build(registry, reportplan.DefaultSelectionPolicy("main", "5.2")); err == nil {
		t.Fatal("report plan accepted divergent development mapping")
	}
	policy, _ := jobpolicy.DefaultPolicy()
	policy.DevelopmentRelease = "5.2"
	if _, err := jobpolicy.Evaluate(registry, policy); err == nil {
		t.Fatal("team policy accepted divergent development mapping")
	}
}

func TestPolicyCommandRejectsConflictingDevelopmentRelease(t *testing.T) {
	policy, _ := jobpolicy.DefaultPolicy()
	policy.DevelopmentRelease = "5.2"
	data, err := yaml.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"job-policy", "--job-registry=../jobregistry/testdata/job-registry.json", "--policy=" + path})
	if err := command.Execute(); err == nil {
		t.Fatal("custom policy diverged from pipeline defaults")
	}
}
