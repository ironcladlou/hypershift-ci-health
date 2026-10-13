import { Fragment, html, groupJobs, jobRelease, releasesInOrder, sippyJobURL, statusClass, WINDOWS } from "../ui.js";
import { useState } from "preact/hooks";
import { RateChart, RateSummary, Sparkline } from "./charts.js";
import { JobDetailsCard } from "./registry.js";
import { JobLinks } from "./job-links.js";
import { Tooltip } from "./tooltip.js";

const registryJobCache = new Map();

function loadRegistryJob(id) {
  if (!registryJobCache.has(id)) {
    const request = fetch(`/api/job-registry/jobs/${encodeURIComponent(id)}`, { headers: { Accept: "application/json" } }).then(response => {
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      return response.json();
    }).catch(error => {
      registryJobCache.delete(id);
      throw error;
    });
    registryJobCache.set(id, request);
  }
  return registryJobCache.get(id);
}

function useRegistryDetail(id, instance) {
  const [expanded, setExpanded] = useState(false);
  const [detail, setDetail] = useState(null);
  const [error, setError] = useState("");
  const panelID = `health-registry-${instance.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
  const toggle = async () => {
    if (expanded) {
      setExpanded(false);
      return;
    }
    setExpanded(true);
    if (detail) return;
    setError("");
    try {
      setDetail(await loadRegistryJob(id));
    } catch (cause) {
      setError(cause.message);
    }
  };
  return { detail, error, expanded, panelID, toggle };
}

function RegistryDetailRow({ registry, dashboards }) {
  if (!registry?.expanded) return null;
  return html`<tr class="registry-detail-row health-registry-detail-row"><td colspan="4">
    <div id=${registry.panelID}>
      ${registry.error
        ? html`<div class="inline-registry-message error">Failed to load registry details: ${registry.error}</div>`
        : registry.detail
          ? html`<${JobDetailsCard} job=${registry.detail} dashboards=${dashboards} />`
          : html`<div class="inline-registry-message">Loading registry details…</div>`}
    </div>
  </td></tr>`;
}

function StatusDot({ rate, runs }) {
  return html`<span class=${`status-dot ${statusClass(rate, runs)}`}></span>`;
}

function InfraBadge({ job, presubmit = false }) {
  if (!job.infra_fails || !job.spark_runs || job.infra_fails / job.spark_runs < 0.15) return null;
  const suffix = presubmit ? " Not caused by PR code." : "";
  return html`<${Tooltip} content=${`${job.infra_fails} of ${job.spark_runs} runs were infrastructure failures.${suffix}`} className="infra-badge">
    ${job.infra_fails} infra
  <//>`;
}

function PeriodicLabel({ job }) {
  const details = [job.relationship_rationale, job.relationship_notes,
    job.relationship_source && `Source: ${job.relationship_source}`,
    job.relationship_verification && `Verification: ${job.relationship_verification}`].filter(Boolean).join(" · ");
  return html`<${Tooltip} content=${details} className="job-type">periodic ${job.label || ""}<//>`;
}

function RateCell({ job, slots, color, error }) {
  if (error) return html`<td class="rate-chart-cell"><${Tooltip} content=${`Sippy analysis could not be retrieved. ${error}`} className="data-unavailable">Data unavailable<//></td>`;
  if (!job.runs) return html`<td class="rate-chart-cell"><span class="rate-na">${job.sippy_ingestion_enabled === false ? "Sippy disabled" : "No recorded runs"}</span></td>`;
  return html`<td class="rate-chart-cell"><div class="rate-chart-inner">
    <${RateChart} data=${job.sparkline} slots=${slots} color=${color} />
    <${RateSummary} job=${job} />
  </div></td>`;
}

function PresubmitRow({ job, slots, window, analysisError }) {
  const hasPeriodics = job.periodics?.length > 0;
  const enabled = job.sippy_ingestion_enabled;
  const registry = useRegistryDetail(job.id || job.prow, `presubmit-${job.id || job.prow}`);
  return html`<${Fragment}>
    <tr class="presubmit-row" data-job-id=${job.id} data-required=${String(job.required)}>
      <td class="job-name">
        <${StatusDot} rate=${job.rate} runs=${job.runs} />
        ${enabled
          ? html`<a href=${sippyJobURL(job.prow, "Presubmits")} target="_blank" rel="noopener">${job.name}</a>`
          : html`<span>${job.name}</span>`}
        <span class="job-type">${job.required ? "Required" : "Optional"} presubmit</span>
        ${!enabled && html`<${Tooltip} content=${job.sippy_ingestion_basis || "Sippy ingestion is disabled."} className="job-type">· Sippy disabled<//>`}
        <${InfraBadge} job=${job} presubmit=${true} />
      </td>
      <${RateCell} job=${job} slots=${slots} color="pre" error=${analysisError("Presubmits", job.prow)} />
      <td class="sparkline-cell"><${Sparkline} data=${job.sparkline} slots=${slots} correlated=${job.correlation?.indices} showDates=${!hasPeriodics} dateEvery=${WINDOWS[window].dateEvery} /></td>
      <${JobLinks} job=${job} release="Presubmits" presubmit=${true} registry=${registry} />
    </tr>
    <${RegistryDetailRow} registry=${registry} dashboards=${["presubmit_health"]} />
    ${(job.periodics || []).map(periodic => html`<${PresubmitPeriodicRow} key=${periodic.id} periodic=${periodic} slots=${slots} window=${window} error=${analysisError(periodic.release, periodic.prow)} />`)}
  <//>`;
}

function PresubmitPeriodicRow({ periodic, slots, window, error }) {
  const registry = useRegistryDetail(periodic.id || periodic.prow, `presubmit-periodic-${periodic.id || periodic.prow}`);
  return html`<${Fragment}>
    <tr class="periodic-row" data-job-id=${periodic.id}>
      <td class="job-name periodic-name">
        <${StatusDot} rate=${periodic.rate} runs=${periodic.runs} />
        <a class="periodic-link" href=${sippyJobURL(periodic.prow, periodic.release)} target="_blank" rel="noopener">${periodic.name}</a>
        <${PeriodicLabel} job=${periodic} /><${InfraBadge} job=${periodic} />
      </td>
      <${RateCell} job=${periodic} slots=${slots} color="per" error=${error} />
      <td class="sparkline-cell"><${Sparkline} data=${periodic.sparkline} slots=${slots} dateEvery=${WINDOWS[window].dateEvery} /></td>
      <${JobLinks} job=${periodic} release=${periodic.release} registry=${registry} />
    </tr>
    <${RegistryDetailRow} registry=${registry} dashboards=${["presubmit_health"]} />
  <//>`;
}

function Participation({ items }) {
  if (!items?.length) return null;
  return html`<div class="perspective-context">${items.map((item, index) => html`<div key=${index}>
    ${item.verification_name || "verification"} ·
    ${item.stream_sippy_url
      ? html`<a href=${item.stream_sippy_url} target="_blank" rel="noopener">${item.stream_name || "release stream"}</a>`
      : item.stream_name || "release stream"}
    ${item.role && html` · ${item.role}`}
    ${item.stream_kind && html` · ${item.stream_kind}`}${item.architecture && html` / ${item.architecture}`}
    ${item.release_status_url && html` · <a href=${item.release_status_url} target="_blank" rel="noopener">payload status</a>`}
  </div>`)}</div>`;
}

function PeriodicHealthRow({ job, slots, window, kind, error }) {
  const registry = useRegistryDetail(job.id || job.prow, `${kind}-${job.release}-${job.id || job.prow}`);
  const availableRegistry = job.registry_missing ? null : registry;
  const dashboards = kind === "payload" ? ["release_payload"] : ["component_readiness"];
  return html`<${Fragment}>
    <tr class="periodic-row">
      <td class="job-name">
        <${StatusDot} rate=${job.rate} runs=${job.runs} />
        <a href=${sippyJobURL(job.prow, job.release)} target="_blank" rel="noopener">${job.name}</a>
        ${kind === "component" && job.registry_missing && html`<${Tooltip} content="Sippy classifies this job as standard, but the generated registry has no current definition." className="infra-badge">registry missing<//>`}
        <${InfraBadge} job=${job} />
        ${kind === "payload" && html`<${Participation} items=${job.participations} />`}
      </td>
      <${RateCell} job=${job} slots=${slots} color="per" error=${error} />
      <td class="sparkline-cell"><${Sparkline} data=${job.sparkline} slots=${slots} dateEvery=${WINDOWS[window].dateEvery} /></td>
      <${JobLinks} job=${job} release=${job.release} registry=${availableRegistry} />
    </tr>
    <${RegistryDetailRow} registry=${availableRegistry} dashboards=${dashboards} />
  <//>`;
}

function Legend({ explainFailures }) {
  return html`<${Fragment}>
    <div class="sparkline-legend">
      <span><i class="legend-good"></i> ≥80% pass</span><span><i class="legend-warning"></i> 50–80%</span>
      <span><i class="legend-critical"></i> ${"<50%"}</span><span><i class="legend-empty"></i> no runs</span>
    </div>
    ${explainFailures && html`<div class="sparkline-legend"><span>fail = tests ran, some failed · infra = job errored before tests completed</span></div>`}
  <//>`;
}

function AlertBanner({ alerts, scope }) {
  if (!alerts?.length) return null;
  const displayed = alerts.slice(0, 8);
  return html`<section class="alert-banner">
    <h2>${scope === "all" ? "Presubmits" : scope === "optional" ? "Optional presubmits" : "Required presubmits"}: ${alerts.length} ${alerts.length === 1 ? "test" : "tests"} newly failing in the last 4 hours</h2>
    <ul>${displayed.map(alert => html`<li key=${alert.test_name}><span class="test-name">${alert.test_name}</span>
      <span class="job-names"> (${alert.failure_count} ${alert.failure_count === 1 ? "failure" : "failures"} in ${alert.jobs.map(job => `${job.name} [${job.required ? "Required" : "Optional"}${job.target_release ? ` · ${job.target_release}` : ""}]`).join(", ")})</span>
    </li>`)}</ul>
    ${alerts.length > 8 && html`<div class="more-alerts">… and ${alerts.length - 8} more</div>`}
  </section>`;
}

export function HealthView({ snapshot, view, group, platforms, selectedReleases, window, focus, onClearFocus, presubmitScope, onPresubmitScope, onIncludeFuture, selectedStreams }) {
  const data = snapshot.data;
  const selected = new Set(selectedReleases);
  const futureRelease = releasesInOrder(snapshot.releases)[0];
  const errors = new Map((snapshot.collection || []).filter(item => item.kind === "analysis" && item.error)
    .map(item => [`${item.release}/${item.prow_job_name}`, item.error]));
  const analysisError = (release, name) => errors.get(`${release}/${name}`);
  let jobs;
  let title;
  let empty;
  let row;
  if (view === "payload") {
    jobs = data.payload_blocking_jobs || [];
    title = "Release payload job";
    empty = "No release payload jobs found for the selected filters.";
    row = job => html`<${PeriodicHealthRow} key=${`${job.id}-${job.release}`} job=${job} slots=${data.sparkline_slots} window=${window} kind="payload" error=${analysisError(job.release, job.prow)} />`;
  } else if (view === "component") {
    jobs = data.component_readiness_jobs || [];
    title = "Component Readiness job";
    empty = "No standard-tier Component Readiness jobs found for the selected filters.";
    row = job => html`<${PeriodicHealthRow} key=${`${job.id}-${job.release}`} job=${job} slots=${data.sparkline_slots} window=${window} kind="component" error=${analysisError(job.release, job.prow)} />`;
  } else {
    jobs = data.jobs || [];
    title = "Job";
    empty = "No presubmit jobs found for the selected filters.";
    row = job => html`<${PresubmitRow} key=${job.id} job=${job} slots=${data.sparkline_slots} window=${window} analysisError=${analysisError} />`;
  }
  const matchesFocus = job => !focus || job.id === focus || job.prow === focus || (job.periodics || []).some(periodic => periodic.id === focus || periodic.prow === focus);
  jobs = jobs.filter(job => {
    if (!matchesFocus(job)) return false;
    if (focus) return true;
    if (view === "presubmit" && presubmitScope !== "all" && job.required !== (presubmitScope === "required")) return false;
    if (view === "payload" && selectedStreams.length && !(job.participations || []).some(item => selectedStreams.includes(item.stream_name))) return false;
    return selected.has(jobRelease(job)) && (!platforms.length || platforms.some(platform => (job.platforms || []).includes(platform)));
  });
  if (view === "payload" && selectedStreams.length && !focus) {
    jobs = jobs.map(job => ({ ...job, participations: job.participations.filter(item => selectedStreams.includes(item.stream_name)) }));
  }
  const visibleIDs = new Set(jobs.map(job => job.id));
  const alerts = (data.alerts || []).map(alert => {
    const matching = alert.jobs.filter(job => visibleIDs.has(job.id));
    return { ...alert, jobs: matching, failure_count: matching.reduce((count, job) => count + job.failure_count, 0) };
  }).filter(alert => alert.jobs.length);
  const futureExcluded = view === "presubmit" && presubmitScope === "optional" && !focus && futureRelease && !selected.has(futureRelease);
  const optionalAvailable = view === "presubmit" && presubmitScope === "required" && !focus && (data.jobs || []).some(job =>
    !job.required && selected.has(jobRelease(job)) && (!platforms.length || platforms.some(platform => job.platforms.includes(platform))));
  if (view === "presubmit") empty = `No ${presubmitScope === "all" ? "" : `${presubmitScope} `}presubmits match the selected filters.`;
  const grouped = groupJobs(jobs, group, releasesInOrder(snapshot.releases));
  return html`<${Fragment}>
    ${focus && html`<div class="job-focus-banner">
      <span>Showing CI Health context for <code>${focus}</code></span>
      <a href=${`/registry?job=${encodeURIComponent(focus)}`}>Registry details</a>
      <button onClick=${onClearFocus}>Show all jobs</button>
    </div>`}
    ${view === "presubmit" && html`<${AlertBanner} alerts=${alerts} scope=${presubmitScope} />`}
    <table>
      <thead><tr><th>${title}</th><th>Pass Rate</th><th>${WINDOWS[window].label}</th><th class="job-links-header">Links</th></tr></thead>
      <tbody>
        ${!jobs.length && html`<tr><td colspan="4" class="loading presubmit-empty">
          ${futureExcluded ? html`<div>Optional presubmits are shown for Future (${futureRelease}) only.</div><button type="button" onClick=${onIncludeFuture}>Include Future (${futureRelease})</button>`
            : html`<div>${empty}</div>${optionalAvailable && html`<div>Optional presubmits match these filters. <button type="button" onClick=${() => onPresubmitScope("optional")}>Show optional</button></div>`}`}
        </td></tr>`}
        ${grouped.map((section, index) => section.heading
          ? html`<${Fragment} key=${index}>
              ${section.dimension === "release" && index > 0 && html`<tr class="release-block-spacer" aria-hidden="true"><td colspan="4"></td></tr>`}
              <tr class=${`category-row ${section.dimension}-group-row`}><td colspan="4">${section.heading}</td></tr>
            <//>`
          : section.subheading
            ? html`<tr class=${`subcategory-row ${section.dimension}-group-row`} key=${index}><td colspan="4">${section.subheading}</td></tr>`
            : section.jobs.map(row))}
      </tbody>
    </table>
    <${Legend} explainFailures=${view === "presubmit"} />
  <//>`;
}
