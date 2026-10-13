# Team job policy

`jobpolicy` is an independent, pure interpretation of registry facts. It ranks
every registry job and assigns team metadata without selecting collection
targets, querying Sippy, or evaluating health. Existing report plans,
observations, health reports, and prefab dashboard views do not consume it.

```console
make generate-job-policy
make generate-job-policy POLICY=/path/to/reviewed-team-policy.yaml
```

The command reads `job-registry.json` and writes `team-job-catalog.json`.
`make generate-artifacts` also generates the catalog as a separate branch of
the pipeline. The binary embeds [the default HyperShift policy](policies/hypershift.yaml),
so deployments do not need a separate configuration file.
Development defaults come from [releaseconfig](../releaseconfig/config.go).
Omitted YAML development fields resolve to those defaults before validation;
the artifact stores explicit `development_branch` and `development_release`
values. This keeps historical evaluations independent of later default changes.
Use `DEVELOPMENT_BRANCH` and `DEVELOPMENT_RELEASE` Make variables to pass the
same override to registry, report-plan, and policy generation. A supplied policy
must declare matching values. Registry branch mappings and served artifact sets
are checked for disagreement; conflicts fail before collection in full generation. `serve --job-policy`
selects a catalog generated with either the default or a custom policy.

## Ordering and assumptions

Groups match in declaration order. The final group must match `all`. Supported
predicates are `release-controller` (explicit stream names and/or stream kinds,
plus effective active role lists, always excluding disabled and end-of-life declarations),
`required-auto-run` (`required && always_run`, retaining other execution
conditions), and `all`.

The development release is the ranking horizon. Jobs whose tested or target
release exceeds it are marked `upcoming` and follow every other job, including
jobs with unknown release scope. Jobs within the horizon and jobs with unknown
scope retain importance-group precedence. Upcoming jobs also retain group
precedence relative to one another. Within a group, dimensions are compared in their
configured order, and stable job ID ascending breaks ties. `release` uses the
presubmit target release when known, resolving development-branch jobs to the
configured development release; otherwise it uses the highest numerically parsed
major.minor value from tested `versions`. `cloud` uses the first recognized cloud in
`cloud_order` after applying explicit, single-step aliases. Unknown dimensions
rank last. Multi-valued input array order does not affect either resolution.

The policy file defines group precedence, dimension order, cloud preferences,
and declared health objectives. If both `streams` and `stream_kinds` are supplied, a single
participation must satisfy both. Multiple participations match independently
of their array order.

Health objectives are declarations only. Defaults are keyed by job type;
group objectives replace the matching type's default, and per-job objectives
replace that result. Named cohorts identify comparison jobs for external
evaluators. The assessment contracts are defined in
[health_objective.go](health_objective.go). No checks run or observed status is
computed by this package.

Team labels and annotations apply to all jobs. Per-job `overrides` may replace
the group/objective, add labels, and merge annotations; a rationale is required.
Override IDs must exist in the registry. For example:

```yaml
overrides:
  exact-prow-job-name:
    rationale: Reviewed critical coverage
    group: hypershift-release-controller
    labels: [critical]
    annotations:
      hypershift/owner: control-plane
```

## Versions and provenance

Policy configuration uses `team-job-policy/v1`; evaluated artifacts and the
public collection use `team-job-catalog/v1`. `job-importance/v1` identifies
evaluation semantics. All versions remain at v1 while the feature is being
prototyped. Input and snapshot digests still distinguish changes to policy and
evaluated output. Version changes will become compatibility boundaries when
the contract is stabilized.

The artifact contains the policy, its digest, the registry digest, algorithm
version, and exactly one ordered reference and evaluated metadata record per
job. Loading rebuilds the expected catalog and rejects mismatched inputs,
unsupported algorithms, altered ranks, and missing or extra entries.

Identical registry, policy, and algorithm inputs produce identical output.
Ranks are one-based global snapshot positions and can shift when jobs change.
The snapshot digest covers the whole artifact; no generation time or live
health state influences it.

## Public API and dashboard

- `GET /api/teams/{team}/jobs` returns every registry job in importance order,
  enriched with `team_metadata`, plus versions and digests. It embeds the exact
  policy for a coherent single-request snapshot.
- `GET /api/teams/{team}/jobs/{id}` returns the same job and metadata record.
- `GET /api/teams/{team}/policy` returns the reviewed policy configuration.

The server serves one configured team catalog at a time. Unknown teams/jobs
return 404; an unavailable catalog returns 503. Collection and individual
responses have content ETags. Existing registry APIs remain unchanged.

`/policy?team=hypershift` is a standalone dashboard perspective. It shares
browser controls and registry detail widgets with `/registry`, but owns its
facets, columns, rank ordering, provenance, and explanations. Filters and
search preserve global rank; name sorting is also available. State and selected
jobs are encoded in the URL. Loading the policy view does not fetch health
windows, and declared objectives have no health status or coloring.
