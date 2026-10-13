import { useMemo } from "preact/hooks";
import { JobLinks } from "./job-links.js";
import { BrowserToolbar, Pagination, SortButton, Tag, Tags, label, useJobBrowser } from "./job-browser.js";
import { Fragment, html } from "../ui.js";

const FACET_ORDER = ["type", "platform", "release", "framework", "repository", "configuration", "payload-role", "dashboard"];
const FACET_LABELS = {
  type: "Type", platform: "Platform", release: "Release", framework: "Framework", repository: "Repository",
  configuration: "Configuration", "payload-role": "Payload role", dashboard: "CI Health",
};
function jobConfigurations(job) {
  const values = [];
  if (job.presubmit) {
    values.push(job.presubmit.required ? "required" : "optional");
    if (job.presubmit.always_run) values.push("always-run");
    values.push(job.presubmit.sippy_ingestion?.enabled ? "sippy-enabled" : "sippy-disabled");
    if (job.presubmit.periodic_counterparts?.length) values.push("has-counterpart");
  }
  return values;
}

function membershipFor(snapshot) {
  const membership = new Map();
  const add = (id, value) => {
    if (!id) return;
    if (!membership.has(id)) membership.set(id, new Set());
    membership.get(id).add(value);
  };
  for (const job of snapshot?.data?.jobs || []) {
    add(job.id || job.prow, "presubmit_health");
    for (const periodic of job.periodics || []) add(periodic.id || periodic.prow, "presubmit_health");
  }
  for (const job of snapshot?.data?.payload_blocking_jobs || []) add(job.id || job.prow, "release_payload");
  for (const job of snapshot?.data?.component_readiness_jobs || []) add(job.id || job.prow, "component_readiness");
  return membership;
}

export function jobFacetValues(job, membership = null) {
  const releases = new Set([...(job.versions || []), job.presubmit?.target_release,
    ...(job.release_controller || []).map(item => item.stream?.release)].filter(Boolean));
  const dashboards = [...(membership?.get(job.id) || [])];
  return {
    type: [job.type],
    platform: job.platforms || [],
    release: [...releases],
    framework: [job.e2e_framework],
    repository: job.repository ? [job.repository] : [],
    configuration: jobConfigurations(job),
    "payload-role": [...new Set((job.release_controller || []).map(item => item.verification?.role).filter(Boolean))],
    ...(membership !== null ? { dashboard: dashboards.length ? dashboards : ["not_selected"] } : {}),
  };
}

function registryJobURL(id, clean = false) {
  if (clean) return `/registry?job=${encodeURIComponent(id)}`;
  const query = new URLSearchParams(location.search);
  query.set("job", id);
  return `/registry?${query}`;
}

function DashboardLinks({ values, id }) {
  const links = {
    presubmit_health: ["/presubmits", "Presubmit Health"],
    release_payload: ["/payload", "Release Payload"],
    component_readiness: ["/component-readiness", "Component Readiness"],
  };
  if (!values.length) return html`<span class="registry-muted">Not selected</span>`;
  return html`<span class="registry-tags">${values.map(value => {
    const item = links[value];
    return html`<a key=${value} class="registry-tag registry-dashboard-tag" href=${`${item[0]}?job=${encodeURIComponent(id)}`}>${item[1]}</a>`;
  })}</span>`;
}

function DetailValue({ label: name, children }) {
  return html`<div><dt>${name}</dt><dd>${children || "—"}</dd></div>`;
}

export function JobDetailsCard({ job, dashboards = null, onRelatedSelect, elementID, policyTeam = null }) {
  const ingestion = job.presubmit?.sippy_ingestion;
  return html`<div class="registry-details" id=${elementID}>
    <section>
      <h3>Identity and source</h3>
      <dl>
        <${DetailValue} label="Stable ID"><code>${job.id}</code><//>
        <${DetailValue} label="Repository">${job.repository}<//>
        <${DetailValue} label="Context">${job.context}<//>
        <${DetailValue} label="Variant">${job.variant}<//>
        <${DetailValue} label="Framework">${label(job.e2e_framework)}<//>
        <${DetailValue} label="Tested releases"><${Tags} values=${job.versions} /><//>
        <${DetailValue} label="Platforms"><${Tags} values=${job.platforms} /><//>
      </dl>
    </section>
    ${job.presubmit && html`<section>
      <h3>Presubmit behavior</h3>
      <dl>
        <${DetailValue} label="Target">${job.presubmit.target_branch} · ${job.presubmit.target_release}<//>
        <${DetailValue} label="Execution">${job.presubmit.required ? "Required" : "Optional"} · ${job.presubmit.always_run ? "Always runs" : "Runs conditionally"}<//>
        <${DetailValue} label="Branches"><${Tags} values=${job.presubmit.branches} /><//>
        <${DetailValue} label="Skip branches"><${Tags} values=${job.presubmit.skip_branches} /><//>
        <${DetailValue} label="Run if changed"><code>${job.presubmit.run_if_changed || "—"}</code><//>
        <${DetailValue} label="Skip if only changed"><code>${job.presubmit.skip_if_only_changed || "—"}</code><//>
      </dl>
    </section>`}
    <section>
      <h3>${dashboards !== null ? "Analysis and dashboard" : "Analysis"}</h3>
      <dl>
        <${DetailValue} label="Sippy ingestion">${ingestion ? `${ingestion.enabled ? "Enabled" : "Disabled"} · ${ingestion.basis}` : "Not applicable"}<//>
        ${dashboards !== null && html`<${DetailValue} label="CI Health"><${DashboardLinks} values=${dashboards} id=${job.id} /><//>`}
      </dl>
    </section>
    ${job.presubmit?.periodic_counterparts?.length ? html`<section class="registry-detail-wide">
      <h3>Periodic counterparts</h3>
      <ul class="registry-relationships">${job.presubmit.periodic_counterparts.map(item => html`<li key=${item.job_id}>
        <a href=${registryJobURL(item.job_id, true)} onClick=${onRelatedSelect ? event => { event.preventDefault(); onRelatedSelect(item.job_id); } : undefined}>${item.job_id}</a>
        <span><${Tag}>${item.tested_release}<//> <${Tag}>${label(item.verification)}<//></span>
        <p>${item.rationale}</p>
        ${item.notes && html`<p>${item.notes}</p>`}
      </li>`)}</ul>
    </section>` : null}
    ${job.release_controller?.length ? html`<section class="registry-detail-wide">
      <h3>Release-controller participation</h3>
      <ul class="registry-relationships">${job.release_controller.map((item, index) => html`<li key=${index}>
        <a href=${item.stream?.release_status_url} target="_blank" rel="noopener">${item.stream?.name || "Unknown stream"}</a>
        <span><${Tag}>${item.verification?.role}<//> <${Tag}>${item.verification?.name}<//></span>
        <p>${item.stream?.kind || "unknown kind"} · ${item.stream?.architecture || "unknown architecture"}${item.stream?.end_of_life ? " · end of life" : ""}
          ${item.source?.url && html` · <a href=${item.source.url} target="_blank" rel="noopener">declaration source</a>`}</p>
      </li>`)}</ul>
    </section>` : null}
    <nav class="registry-detail-actions" aria-label=${`Links for ${job.name}`}>
      ${job.prow_job_history_url && html`<a href=${job.prow_job_history_url} target="_blank" rel="noopener">Prow history</a>`}
      ${job.sippy_url && html`<a href=${job.sippy_url} target="_blank" rel="noopener">Sippy analysis</a>`}
      ${job.source?.url && html`<a href=${job.source.url} target="_blank" rel="noopener">Source definition</a>`}
      <a href=${`/api/job-registry/jobs/${encodeURIComponent(job.id)}`} target="_blank" rel="noopener">Raw API</a>
      ${policyTeam !== null && html`<a href=${`/policy?team=${encodeURIComponent(policyTeam)}&job=${encodeURIComponent(job.id)}`}>View in Job Policy</a>`}
      <a href="/api/schemas/Job.json" target="_blank" rel="noopener">JSON Schema</a>
    </nav>
  </div>`;
}

function RegistryEntry({ job, team, dashboards, expanded, onSelect, onRelatedSelect }) {
  const configurations = jobConfigurations(job);
  const toggle = event => {
    event.preventDefault();
    onSelect(expanded ? "" : job.id);
  };
  return html`<${Fragment}>
    <tr class=${`registry-job ${expanded ? "expanded" : ""}`} id=${`registry-job-${job.id}`}>
      <td class="job-name">
        <a class="registry-job-link" href=${registryJobURL(job.id)} aria-expanded=${expanded} aria-controls=${`registry-details-${job.id}`} onClick=${toggle}>${job.name}</a>
        <div class="registry-row-meta"><${Tag}>${job.type}<//>${job.context && html`<span>${job.context}</span>`}</div>
      </td>
      <td><div><${Tags} values=${job.versions} /></div><div class="registry-row-secondary"><${Tags} values=${job.platforms} /></div></td>
      <td><${Tags} values=${configurations} empty="—" /></td>
      <td><${DashboardLinks} values=${dashboards} id=${job.id} /></td>
      <${JobLinks} job=${job} registry=${{ expanded, panelID: `registry-details-${job.id}`, toggle: () => onSelect(expanded ? "" : job.id) }} />
    </tr>
    ${expanded && html`<tr class="registry-detail-row"><td colspan="5"><${JobDetailsCard} job=${job} policyTeam=${team} dashboards=${dashboards} onRelatedSelect=${onRelatedSelect} elementID=${`registry-details-${job.id}`} /></td></tr>`}
  <//>`;
}

export function RegistryView({ registry, team = "hypershift", snapshot, query, selectedJob, filters, sort, order, page, onState }) {
  const jobs = useMemo(() => registry.jobs || [], [registry]);
  const membership = useMemo(() => membershipFor(snapshot), [snapshot]);
  const facetValues = useMemo(() => new Map(jobs.map(job => [job.id, jobFacetValues(job, membership)])), [jobs, membership]);
  const searchable = useMemo(() => jobs.map(job => {
    const presubmit = job.presubmit || {};
    return {
      job, id: job.id, name: job.name || "", type: job.type || "", repository: job.repository || "", context: job.context || "",
      variant: job.variant || "", framework: job.e2e_framework || "", versions: (job.versions || []).join(" "), platforms: (job.platforms || []).join(" "),
      target: [presubmit.target_branch, presubmit.target_release].filter(Boolean).join(" "), configuration: jobConfigurations(job).join(" "),
      sippy: presubmit.sippy_ingestion?.basis || "",
      counterparts: (presubmit.periodic_counterparts || []).flatMap(item => [item.job_id, item.tested_release, item.source, item.verification, item.rationale]).join(" "),
      releases: (job.release_controller || []).flatMap(item => [item.stream?.name, item.stream?.release, item.stream?.architecture,
        item.verification?.name, item.verification?.role]).filter(Boolean).join(" "),
    };
  }), [jobs]);
  const sortValue = job => {
    const values = facetValues.get(job.id);
    if (sort === "type") return job.type;
    if (sort === "scope") return [...values.release, ...values.platform].join(" ");
    if (sort === "configuration") return values.configuration.join(" ");
    if (sort === "dashboard") return values.dashboard.join(" ");
    return job.name || job.id;
  };
  const { pageJobs, visible, currentPage, pages, options, setFilter, clearFilters, resetAll } = useJobBrowser({
    jobs, facetValues, searchable, facetNames: FACET_ORDER, query, filters, sort, order, page, selectedJob, onState,
    compare: (left, right) => sortValue(left).localeCompare(sortValue(right), undefined, { numeric: true }),
    promoteExact: true,
  });
  const selectRelated = id => onState({ search: "", registryFilters: Object.fromEntries(FACET_ORDER.map(name => [name, []])), registryPage: 1, selectedJob: id });
  const setSort = value => onState({ registrySort: value, registryOrder: sort === value && order === "asc" ? "desc" : "asc", registryPage: 1 });

  return html`<${Fragment}>
    <${BrowserToolbar} query=${query} total=${jobs.length} count=${visible.length} facetNames=${FACET_ORDER}
      facetLabels=${FACET_LABELS} options=${options} filters=${filters} onState=${onState}
      setFilter=${setFilter} clearFilters=${clearFilters} searchLabel="Search job registry" />
    <${Pagination} page=${currentPage} pages=${pages} total=${visible.length} onPage=${registryPage => onState({ registryPage, selectedJob: "" })} />
    <table class="registry-table">
      <thead><tr>
        <th><${SortButton} value="name" active=${sort} order=${order} onSort=${setSort}>Job<//></th>
        <th><${SortButton} value="scope" active=${sort} order=${order} onSort=${setSort}>Releases / platforms<//></th>
        <th><${SortButton} value="configuration" active=${sort} order=${order} onSort=${setSort}>Configuration<//></th>
        <th><${SortButton} value="dashboard" active=${sort} order=${order} onSort=${setSort}>CI Health<//></th>
        <th class="job-links-header" aria-label="Job links"></th>
      </tr></thead>
      <tbody>
        ${!pageJobs.length && html`<tr><td colspan="5" class="registry-empty"><strong>No jobs match these criteria.</strong><span>Try a broader search or clear the registry filters.</span><button onClick=${resetAll}>Clear search and filters</button></td></tr>`}
        ${pageJobs.map(job => html`<${RegistryEntry} key=${job.id} job=${job} team=${team} dashboards=${[...(membership.get(job.id) || [])]} expanded=${selectedJob === job.id} onSelect=${id => onState({ selectedJob: id })} onRelatedSelect=${selectRelated} />`)}
      </tbody>
    </table>
    <${Pagination} page=${currentPage} pages=${pages} total=${visible.length} onPage=${registryPage => onState({ registryPage, selectedJob: "" })} />
  <//>`;
}
