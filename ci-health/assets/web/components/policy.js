import { useMemo } from "preact/hooks";
import { Fragment, html } from "../ui.js";
import { BrowserToolbar, Pagination, SortButton, Tags, useJobBrowser } from "./job-browser.js";
import { JobDetailsCard, jobFacetValues } from "./registry.js";
import { JobLinks } from "./job-links.js";

const FACETS = ["type", "platform", "release", "framework", "repository", "configuration", "payload-role", "release-scope", "presubmit-branch", "importance-group", "team-label", "override"];
const SCOPE_LABELS = { "in-horizon": "Within release horizon", upcoming: "Upcoming", unknown: "Unknown release scope" };
const BRANCH_LABELS = { development: "Development branch", release: "Release branch", prospective: "Prospective release branch", unknown: "Unknown target branch" };
const LABELS = {
  type: "Type", platform: "Platform", release: "Release", framework: "Framework", repository: "Repository",
  configuration: "Configuration", "payload-role": "Payload role", "release-scope": "Release scope", "importance-group": "Importance group", "team-label": "Team label", override: "Override",
  "presubmit-branch": "Presubmit branch",
};

function objectiveLabel(objective) {
  const passRate = objective.checks.pass_rate;
  return passRate ? `Pass rate ≥${passRate.minimum_pass_rate_bps / 100}% / ${passRate.window}` : "No systemic failures";
}

function objectiveSummary(objective) {
  const checks = objective.checks;
  return checks.pass_rate ? `Minimum ${checks.pass_rate.minimum_runs} completed runs`
    : `Permafailing ${checks.job_permafailing.window} · Correlated failures ${checks.correlated_failures.window}`;
}

function HealthObjectiveDetails({ objective, catalog }) {
  const checks = objective.checks;
  const cohort = checks.correlated_failures && catalog.policy.cohorts[checks.correlated_failures.cohort];
  return html`<div class="health-objective-details">
    <div>${objectiveLabel(objective)} · not evaluated</div>
    <dl>
      <dt>Assessment contract</dt><dd>${objective.assessment}</dd>
      ${checks.pass_rate && html`<dt>Pass-rate check</dt><dd>At least ${checks.pass_rate.minimum_pass_rate_bps / 100}% passing over ${checks.pass_rate.window}, with at least ${checks.pass_rate.minimum_runs} completed, non-aborted runs. Infrastructure errors count as failures.</dd>`}
      ${checks.job_permafailing && html`<dt>Job permafailing</dt><dd>Over ${checks.job_permafailing.window}: at least ${checks.job_permafailing.minimum_distinct_prs} distinct PRs, with at most ${checks.job_permafailing.maximum_successful_prs} successful PRs. Reruns do not count as independent PRs.</dd>`}
      ${checks.correlated_failures && html`<dt>Correlated failures</dt><dd>Over ${checks.correlated_failures.window}: a shared test or infrastructure-error signature across at least ${checks.correlated_failures.minimum_distinct_prs} distinct PRs and ${checks.correlated_failures.minimum_distinct_jobs} jobs.</dd>
        <dt>Comparison cohort</dt><dd>${checks.correlated_failures.cohort} · ${cohort.job_type} jobs from this team, ${cohort.target_branch === "same-as-job" ? "on the same target branch" : `on development branch ${catalog.policy.development_branch}`}, release scope ${cohort.release_scopes.join(", ")}${cohort.exclude_presubmit_branch_scopes?.length ? `; excludes ${cohort.exclude_presubmit_branch_scopes.join(", ")} branches` : ""}.</dd>`}
      ${objective.attribution && html`<dt>Required attribution</dt><dd>Failures must be supported by evidence that they are unrelated to PR contents. Failure counts alone are insufficient.</dd>`}
      <dt>Insufficient evidence</dt><dd>Indeterminate; never assumed satisfied.</dd>
    </dl>
  </div>`;
}

function PolicyEntry({ job, metadata, catalog, expanded, onSelect, onRelatedSelect }) {
  const importance = metadata.importance;
  const query = new URLSearchParams(location.search);
  query.set("job", job.id);
  const base = `/api/teams/${encodeURIComponent(catalog.team)}`;
  return html`<${Fragment}>
    <tr class=${`registry-job ${expanded ? "expanded" : ""}`} id=${`registry-job-${job.id}`}>
      <td class="policy-rank">${importance.rank}</td>
      <td class="job-name"><a class="registry-job-link" href=${`/policy?${query}`} aria-expanded=${expanded}
        aria-controls=${`registry-details-${job.id}`} onClick=${event => { event.preventDefault(); onSelect(expanded ? "" : job.id); }}>${job.name}</a>
        <div class="registry-row-meta">${job.type}${metadata.labels.length ? html`<${Tags} values=${metadata.labels} />` : null}</div></td>
      <td>${importance.group}<div class="registry-row-secondary">${SCOPE_LABELS[importance.release_scope]}</div>${importance.presubmit_branch_scope && html`<div class="registry-row-secondary">${BRANCH_LABELS[importance.presubmit_branch_scope]}</div>`}${metadata.override_rationale && html`<div class="registry-row-secondary">Override</div>`}</td>
      <td><div>${importance.resolved_release || "Unknown release"}</div><div class="registry-row-secondary">${importance.resolved_cloud || "Unknown cloud"}</div></td>
      <td>${objectiveLabel(metadata.health_objective)}<div class="registry-row-secondary">${objectiveSummary(metadata.health_objective)}</div></td>
      <${JobLinks} job=${job} registry=${{ expanded, panelID: `registry-details-${job.id}`, toggle: () => onSelect(expanded ? "" : job.id) }} />
    </tr>
    ${expanded && html`<tr class="registry-detail-row"><td colspan="6">
      <div class="policy-explanation">
        <h3>Policy explanation</h3>
        <p>Global rank ${importance.rank}. Upcoming jobs follow all other jobs. Within each part, importance group takes precedence, followed by ${catalog.policy.dimensions.join(" then ")}; job ID breaks ties.</p>
        ${importance.presubmit_branch_scope === "prospective" && html`<p>This release-branch definition targets the current or a later development release. It ranks after active jobs because development currently uses ${catalog.policy.development_branch} for ${catalog.policy.development_release}.</p>`}
        <dl>
          <dt>Release scope</dt><dd>${SCOPE_LABELS[importance.release_scope]} · ${importance.scope_release || "unknown"} compared with development release ${catalog.policy.development_release}; uses tested and target releases</dd>
          <dt>Matched rules</dt><dd>${importance.matched_rules.map(id => catalog.policy.groups.find(group => group.id === id)?.description || id).join("; ")}</dd>
          ${job.presubmit && html`<dt>Target branch</dt><dd>${job.presubmit.target_branch || "Unknown"} · ${BRANCH_LABELS[importance.presubmit_branch_scope]} · required and always-run flags apply to matching target branches</dd>`}
          ${job.type === "presubmit" && catalog.policy.dimensions.includes("non-e2e-presubmit") && html`<dt>Framework priority</dt><dd>${job.e2e_framework === "none" ? "Non-E2E presubmit; preferred at the framework step of ranking" : "E2E or unspecified framework; no framework preference"}</dd>`}
          ${job.type === "periodic" && catalog.policy.dimensions.includes("required-presubmit-periodic") && html`<dt>Required presubmit counterparts</dt><dd>${importance.required_presubmit_ids?.length
            ? importance.required_presubmit_ids.map(id => html`<div key=${id}><a href=${`/policy?team=${encodeURIComponent(catalog.team)}&job=${encodeURIComponent(id)}`} onClick=${event => { event.preventDefault(); onRelatedSelect(id); }}>${id}</a></div>`)
            : "No verified relationship to a required presubmit"}</dd>`}
          <dt>Ranking release</dt><dd>${importance.resolved_release || "Unknown"} · ${job.presubmit && (job.presubmit.target_release || importance.presubmit_branch_scope === "development") ? "presubmit target release" : "highest concrete tested release"}</dd>
          <dt>Ranking cloud</dt><dd>${importance.resolved_cloud || "Unknown"} · first recognized cloud in ${catalog.policy.cloud_order.join(" → ")}</dd>
          <dt>Health objective</dt><dd><${HealthObjectiveDetails} objective=${metadata.health_objective} catalog=${catalog} /></dd>
          ${metadata.override_rationale && html`<dt>Override rationale</dt><dd>${metadata.override_rationale}</dd>`}
          ${Object.entries(metadata.annotations).map(([key, value]) => html`<dt key=${key}>${key}</dt><dd>${value}</dd>`)}
        </dl>
        <nav class="registry-detail-actions" aria-label="Policy links">
          <a href=${`/registry?team=${encodeURIComponent(catalog.team)}&job=${encodeURIComponent(job.id)}`}>View in Job Registry</a>
          <a href=${`${base}/jobs/${encodeURIComponent(job.id)}`} target="_blank" rel="noopener">Policy API</a>
        </nav>
      </div>
      <${JobDetailsCard} job=${job} onRelatedSelect=${onRelatedSelect} elementID=${`registry-details-${job.id}`} />
    </td></tr>`}
  <//>`;
}

export function JobPolicyView({ catalog, query, selectedJob, filters, sort, order, page, onState }) {
  const jobs = useMemo(() => catalog.jobs.map(entry => entry.job), [catalog]);
  const metadata = useMemo(() => new Map(catalog.jobs.map(entry => [entry.job.id, entry.team_metadata])), [catalog]);
  const facetValues = useMemo(() => new Map(jobs.map(job => {
    const values = metadata.get(job.id);
    return [job.id, { ...jobFacetValues(job), "release-scope": [values.importance.release_scope], "presubmit-branch": values.importance.presubmit_branch_scope ? [values.importance.presubmit_branch_scope] : [], "importance-group": [values.importance.group], "team-label": values.labels,
      override: [values.override_rationale ? "Overridden" : "No override"] }];
  })), [jobs, metadata]);
  const searchable = useMemo(() => jobs.map(job => ({
    job, id: job.id, name: job.name, repository: job.repository || "", context: job.context || "", variant: job.variant || "",
    dimensions: Object.values(facetValues.get(job.id)).flat().join(" "),
    rules: metadata.get(job.id).importance.matched_rules.join(" "),
    annotations: Object.entries(metadata.get(job.id).annotations).flat().join(" "),
    rationale: metadata.get(job.id).override_rationale || "",
  })), [jobs, metadata, facetValues]);
  const { pageJobs, visible, currentPage, pages, options, setFilter, clearFilters, resetAll } = useJobBrowser({
    jobs, facetValues, searchable, facetNames: FACETS, query, filters, sort, order, page, selectedJob, onState,
    compare: (a, b) => sort === "name" ? a.name.localeCompare(b.name, undefined, { numeric: true })
      : metadata.get(a.id).importance.rank - metadata.get(b.id).importance.rank,
  });
  const selectRelated = id => onState({ search: "", registryFilters: {}, registryPage: 1, selectedJob: id });
  const setSort = value => onState({ registrySort: value, registryOrder: sort === value && order === "asc" ? "desc" : "asc", registryPage: 1, selectedJob: "" });
  return html`<${Fragment}>
    <section class="policy-context" aria-label="Active team policy">
      <h2>Job Policy · ${catalog.team}</h2>
      <p>${catalog.policy_version} · ${catalog.algorithm_version} · Development release ${catalog.policy.development_release}. Declared health objectives are not evaluated.</p>
      <details><summary>Policy definition and provenance</summary>
        <p>Jobs beyond development release ${catalog.policy.development_release} rank after all other jobs. Development branch: ${catalog.policy.development_branch}.</p>
        <p>Prospective release-branch presubmits follow active jobs. Presubmit release weighting uses the target release; periodics use the tested release.</p>
        <p>Group order: ${catalog.policy.groups.map(group => group.id).join(" → ")}. Within groups: ${catalog.policy.dimensions.join(" → ")} → job ID.</p>
        <p>Cloud aliases: ${Object.entries(catalog.policy.cloud_aliases || {}).map(([alias, cloud]) => `${alias} → ${cloud}`).join(", ") || "None"}. Unknown dimensions rank last.</p>
        <dl><dt>Registry digest</dt><dd><code>${catalog.registry_digest}</code></dd><dt>Policy digest</dt><dd><code>${catalog.policy_digest}</code></dd>
          <dt>Snapshot digest</dt><dd><code>${catalog.snapshot_digest}</code></dd></dl>
        <p><a href=${`/api/teams/${encodeURIComponent(catalog.team)}/policy`} target="_blank" rel="noopener">Policy definition API</a></p>
      </details>
    </section>
    <${BrowserToolbar} query=${query} total=${jobs.length} count=${visible.length} facetNames=${FACETS} facetLabels=${LABELS}
      options=${options} filters=${filters} onState=${onState} setFilter=${setFilter} clearFilters=${clearFilters} searchLabel="Search job policy" />
    <${Pagination} page=${currentPage} pages=${pages} total=${visible.length} onPage=${registryPage => onState({ registryPage, selectedJob: "" })} />
    <table class="registry-table policy-table">
      <thead><tr>
        <th><${SortButton} value="rank" active=${sort} order=${order} onSort=${setSort}>Rank<//></th>
        <th><${SortButton} value="name" active=${sort} order=${order} onSort=${setSort}>Job<//></th>
        <th>Importance group</th><th>Ranking release / cloud</th><th>Health objective</th>
        <th class="job-links-header" aria-label="Job links"></th>
      </tr></thead>
      <tbody>
        ${!pageJobs.length && html`<tr><td colspan="6" class="registry-empty"><strong>No jobs match these criteria.</strong><button onClick=${resetAll}>Clear search and filters</button></td></tr>`}
        ${pageJobs.map(job => html`<${PolicyEntry} key=${job.id} job=${job} metadata=${metadata.get(job.id)} catalog=${catalog}
          expanded=${selectedJob === job.id} onSelect=${selectedJob => onState({ selectedJob })} onRelatedSelect=${selectRelated} />`)}
      </tbody>
    </table>
    <${Pagination} page=${currentPage} pages=${pages} total=${visible.length} onPage=${registryPage => onState({ registryPage, selectedJob: "" })} />
  <//>`;
}
