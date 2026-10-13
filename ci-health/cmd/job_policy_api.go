package cmd

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobpolicy"
	"github.com/ironcladlou/hypershift-ci-health/ci-health/jobregistry"
)

type teamJob struct {
	Job          jobregistry.Job    `json:"job"`
	TeamMetadata jobpolicy.Metadata `json:"team_metadata"`
}

type teamCatalog struct {
	Policy           jobpolicy.Policy `json:"policy" doc:"Policy embedded with this snapshot to avoid joining evaluations from different deployments"`
	APIVersion       string           `json:"api_version"`
	Team             string           `json:"team"`
	AlgorithmVersion string           `json:"algorithm_version"`
	PolicyVersion    string           `json:"policy_version"`
	PolicyDigest     string           `json:"policy_digest"`
	RegistryDigest   string           `json:"registry_digest"`
	SnapshotDigest   string           `json:"snapshot_digest"`
	Jobs             []teamJob        `json:"jobs" doc:"Every registry job exactly once in global importance order"`
}

func (state *applicationState) setJobPolicy(catalog *jobpolicy.Catalog) error {
	if err := catalog.Validate(state.registry); err != nil {
		return err
	}
	digest, err := jobpolicy.Digest(catalog)
	if err != nil {
		return err
	}
	response := &teamCatalog{Policy: catalog.Policy, APIVersion: catalog.APIVersion, Team: catalog.Policy.Team, AlgorithmVersion: catalog.AlgorithmVersion, PolicyVersion: catalog.Policy.Version, PolicyDigest: catalog.PolicyDigest, RegistryDigest: catalog.RegistryDigest, SnapshotDigest: digest, Jobs: []teamJob{}}
	state.teamJobIndex = map[string]*teamJob{}
	for _, entry := range catalog.Jobs {
		response.Jobs = append(response.Jobs, teamJob{Job: *state.registryIndex[entry.JobID], TeamMetadata: entry.TeamMetadata})
	}
	for i := range response.Jobs {
		state.teamJobIndex[response.Jobs[i].Job.ID] = &response.Jobs[i]
	}
	state.jobPolicy = catalog
	state.teamCatalog = response
	base := "/api/teams/" + catalog.Policy.Team
	state.policyETags = map[string]string{base + "/jobs": contentETag(response), base + "/policy": contentETag(catalog.Policy)}
	for id, job := range state.teamJobIndex {
		state.policyETags[base+"/jobs/"+id] = contentETag(job)
	}
	return nil
}

type getTeamInput struct {
	Team string `path:"team" doc:"Team identifier"`
}
type getTeamJobInput struct {
	Team string `path:"team"`
	ID   string `path:"id" doc:"Globally unique Prow job identifier"`
}
type getTeamCatalogOutput struct{ Body teamCatalog }
type getTeamJobOutput struct{ Body teamJob }
type getTeamPolicyOutput struct{ Body jobpolicy.Policy }

func registerJobPolicyAPI(api huma.API, state *applicationState) {
	checkTeam := func(team string) error {
		if state.teamCatalog == nil {
			return huma.Error503ServiceUnavailable("team job catalog unavailable")
		}
		if team != state.teamCatalog.Team {
			return huma.Error404NotFound(fmt.Sprintf("team %q not found", team))
		}
		return nil
	}
	huma.Register(api, huma.Operation{OperationID: "get-team-jobs", Method: "GET", Path: "/api/teams/{team}/jobs", Summary: "Get every job in team importance order", Description: "Returns the latest deployed team policy evaluation. Ordering depends only on registry facts, policy, and algorithm version. Structured health objectives declare assessment contracts, checks, evidence requirements, and cohort references for external evaluators. This API does not evaluate health or collect health data.", Tags: []string{"Job policy"}, Errors: []int{404, 503}}, func(_ context.Context, input *getTeamInput) (*getTeamCatalogOutput, error) {
		if err := checkTeam(input.Team); err != nil {
			return nil, err
		}
		return &getTeamCatalogOutput{Body: *state.teamCatalog}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "get-team-job", Method: "GET", Path: "/api/teams/{team}/jobs/{id}", Summary: "Get a job and its evaluated team metadata", Tags: []string{"Job policy"}, Errors: []int{404, 503}}, func(_ context.Context, input *getTeamJobInput) (*getTeamJobOutput, error) {
		if err := checkTeam(input.Team); err != nil {
			return nil, err
		}
		job := state.teamJobIndex[input.ID]
		if job == nil {
			return nil, huma.Error404NotFound("job not found")
		}
		return &getTeamJobOutput{Body: *job}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "get-team-policy", Method: "GET", Path: "/api/teams/{team}/policy", Summary: "Get the reviewed team policy used by this catalog", Tags: []string{"Job policy"}, Errors: []int{404, 503}}, func(_ context.Context, input *getTeamInput) (*getTeamPolicyOutput, error) {
		if err := checkTeam(input.Team); err != nil {
			return nil, err
		}
		return &getTeamPolicyOutput{Body: state.jobPolicy.Policy}, nil
	})
}
