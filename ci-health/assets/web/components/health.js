import { Fragment, html, groupJobs, jobRelease, releasesInOrder, sippyJobURL, statusClass, WINDOWS } from "../ui.js";
import { RateChart, RateSummary, Sparkline } from "./charts.js";
import { Tooltip } from "./tooltip.js";

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

function SippyIcon() {
  return html`<svg focusable="false" aria-hidden="true" viewBox="0 0 100 100">
    <path d="M 32 28 C 12 28 5 42 10 59" stroke="#848484" stroke-width="9" stroke-linecap="round" fill="none" />
    <path d="M 68 28 C 88 28 95 42 90 59" stroke="#848484" stroke-width="9" stroke-linecap="round" fill="none" />
    <path d="M 24 40 H 76 C 76 40 82 54 82 70 C 82 86 70 93 50 93 C 30 93 18 86 18 70 C 18 54 24 40 24 40 Z" fill="#b7b7b7" />
    <path d="M 24 40 H 76 C 76 42 70 48 50 48 C 30 48 24 42 24 40 Z" fill="#929292" />
    <path d="M 27 53 H 73 C 74 53 75 64 75 72 C 75 83 64 87 50 87 C 36 87 25 83 25 72 C 25 64 26 53 27 53 Z" fill="#bebebe" />
    <ellipse cx="37" cy="68" rx="2.2" ry="3.8" fill="#3b3b3b" />
    <ellipse cx="63" cy="68" rx="2.2" ry="3.8" fill="#3b3b3b" />
    <path d="M 45 71 Q 50 75 55 71" stroke="#3b3b3b" stroke-width="2.5" stroke-linecap="round" fill="none" />
    <path d="M 50 7 C 45 7 38 18 33 26 C 44 28 56 28 67 26 C 62 18 55 7 50 7 Z" fill="#747474" />
    <path d="M 19 36 C 19 25 33 23 50 23 C 67 23 81 25 81 36 C 81 42 67 44 50 44 C 33 44 19 42 19 36 Z" fill="#848484" />
  </svg>`;
}

function ProwIcon() {
  return html`<svg focusable="false" aria-hidden="true" viewBox="0 0 24 24" fill="currentColor">
    <path d="M20 21c-1.39 0-2.78-.47-4-1.32-2.44 1.71-5.56 1.71-8 0C6.78 20.53 5.39 21 4 21H2v2h2c1.38 0 2.74-.35 4-.99 2.52 1.29 5.48 1.29 8 0 1.26.65 2.62.99 4 .99h2v-2h-2zM3.95 19H4c1.6 0 3.02-.88 4-2 .98 1.12 2.4 2 4 2s3.02-.88 4-2c.98 1.12 2.4 2 4 2h.05l1.89-6.68c.08-.26.06-.54-.06-.78s-.34-.42-.6-.5L20 10.62V6c0-1.1-.9-2-2-2h-3V1H9v3H6c-1.1 0-2 .9-2 2v4.62l-1.29.42c-.26.08-.48.26-.6.5s-.15.52-.06.78L3.95 19zM6 6h12v3.97L12 8 6 9.97V6z" />
  </svg>`;
}

function JobLinks({ job, release, presubmit = false }) {
  const sippy = !presubmit || job.sippy_ingestion_enabled ? sippyJobURL(job.prow, release) : "";
  return html`<td class="job-links">
    ${sippy && html`<a class="job-link-icon" href=${sippy} target="_blank" rel="noopener" title="Sippy analysis" aria-label=${`Sippy analysis for ${job.prow}`}><${SippyIcon} /></a>`}
    ${job.prow_job_history_url && html`<a class="job-link-icon" href=${job.prow_job_history_url} target="_blank" rel="noopener" title="Prow job history" aria-label=${`Prow history for ${job.prow}`}><${ProwIcon} /></a>`}
  </td>`;
}

function PeriodicLabel({ job }) {
  const details = [job.relationship_rationale,
    job.relationship_source && `Source: ${job.relationship_source}`,
    job.relationship_verification && `Verification: ${job.relationship_verification}`].filter(Boolean).join(" · ");
  const review = job.relationship_verification === "needs-review" ? " · needs review" : "";
  return html`<${Tooltip} content=${details} className="job-type">periodic ${job.label || ""}${review}<//>`;
}

function RateCell({ job, slots, color }) {
  return html`<td class="rate-chart-cell"><div class="rate-chart-inner">
    <${RateChart} data=${job.sparkline} slots=${slots} color=${color} />
    <${RateSummary} job=${job} />
  </div></td>`;
}

function PresubmitRow({ job, slots, window }) {
  const hasPeriodics = job.periodics?.length > 0;
  const enabled = job.sippy_ingestion_enabled;
  return html`<${Fragment}>
    <tr>
      <td class="job-name">
        <${StatusDot} rate=${job.rate} runs=${job.runs} />
        ${enabled
          ? html`<a href=${sippyJobURL(job.prow, "Presubmits")} target="_blank" rel="noopener">${job.name}</a>`
          : html`<span>${job.name}</span>`}
        <span class="job-type">presubmit</span>
        ${!enabled && html`<${Tooltip} content=${job.sippy_ingestion_basis || "Sippy ingestion is disabled."} className="job-type">· Sippy disabled<//>`}
        <${InfraBadge} job=${job} presubmit=${true} />
      </td>
      <${RateCell} job=${job} slots=${slots} color="pre" />
      <td class="sparkline-cell"><${Sparkline} data=${job.sparkline} slots=${slots} correlated=${job.correlation?.indices} showDates=${!hasPeriodics} dateEvery=${WINDOWS[window].dateEvery} /></td>
      <${JobLinks} job=${job} release="Presubmits" presubmit=${true} />
    </tr>
    ${(job.periodics || []).map(periodic => html`<tr class="periodic-row" key=${periodic.id}>
      <td class="job-name periodic-name">
        <${StatusDot} rate=${periodic.rate} runs=${periodic.runs} />
        <a class="periodic-link" href=${sippyJobURL(periodic.prow, periodic.release)} target="_blank" rel="noopener">${periodic.name}</a>
        <${PeriodicLabel} job=${periodic} /><${InfraBadge} job=${periodic} />
      </td>
      <${RateCell} job=${periodic} slots=${slots} color="per" />
      <td class="sparkline-cell"><${Sparkline} data=${periodic.sparkline} slots=${slots} dateEvery=${WINDOWS[window].dateEvery} /></td>
      <${JobLinks} job=${periodic} release=${periodic.release} />
    </tr>`)}
  <//>`;
}

function Participation({ items }) {
  if (!items?.length) return null;
  return html`<div class="perspective-context">${items.map((item, index) => html`<div key=${index}>
    ${item.verification_name || "verification"} ·
    ${item.stream_sippy_url
      ? html`<a href=${item.stream_sippy_url} target="_blank" rel="noopener">${item.stream_name || "release stream"}</a>`
      : item.stream_name || "release stream"}
    · ${item.stream_kind || "stream"} / ${item.architecture || "unknown architecture"}
    ${item.release_status_url && html` · <a href=${item.release_status_url} target="_blank" rel="noopener">payload status</a>`}
  </div>`)}</div>`;
}

function PeriodicHealthRow({ job, slots, window, kind }) {
  return html`<tr class="periodic-row">
    <td class="job-name">
      <${StatusDot} rate=${job.rate} runs=${job.runs} />
      <a href=${sippyJobURL(job.prow, job.release)} target="_blank" rel="noopener">${job.name}</a>
      ${kind === "component" && job.registry_missing && html`<${Tooltip} content="Sippy classifies this job as standard, but the generated registry has no current definition." className="infra-badge">registry missing<//>`}
      <${InfraBadge} job=${job} />
      ${kind === "payload" && html`<${Participation} items=${job.participations} />`}
    </td>
    <${RateCell} job=${job} slots=${slots} color="per" />
    <td class="sparkline-cell"><${Sparkline} data=${job.sparkline} slots=${slots} dateEvery=${WINDOWS[window].dateEvery} /></td>
    <${JobLinks} job=${job} release=${job.release} />
  </tr>`;
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

function AlertBanner({ alerts }) {
  if (!alerts?.length) return null;
  const displayed = alerts.slice(0, 8);
  return html`<section class="alert-banner">
    <h2>${alerts.length} ${alerts.length === 1 ? "test" : "tests"} newly failing in the last 4 hours</h2>
    <ul>${displayed.map(alert => html`<li key=${alert.test_name}><span class="test-name">${alert.test_name}</span>
      <span class="job-names"> (${alert.failure_count} ${alert.failure_count === 1 ? "failure" : "failures"}${alert.jobs?.length ? ` in ${alert.jobs.join(", ")}` : ""})</span>
    </li>`)}</ul>
    ${alerts.length > 8 && html`<div class="more-alerts">… and ${alerts.length - 8} more</div>`}
  </section>`;
}

export function HealthView({ snapshot, view, group, platforms, selectedReleases, window }) {
  const data = snapshot.data;
  const selected = new Set(selectedReleases);
  let jobs;
  let title;
  let empty;
  let row;
  if (view === "payload") {
    jobs = data.payload_blocking_jobs || [];
    title = "Release payload job";
    empty = "No release payload jobs found for the selected filters.";
    row = job => html`<${PeriodicHealthRow} key=${`${job.id}-${job.release}`} job=${job} slots=${data.sparkline_slots} window=${window} kind="payload" />`;
  } else if (view === "component") {
    jobs = data.component_readiness_jobs || [];
    title = "Component Readiness job";
    empty = "No standard-tier Component Readiness jobs found for the selected filters.";
    row = job => html`<${PeriodicHealthRow} key=${`${job.id}-${job.release}`} job=${job} slots=${data.sparkline_slots} window=${window} kind="component" />`;
  } else {
    jobs = data.jobs || [];
    title = "Job";
    empty = "No presubmit jobs found for the selected filters.";
    row = job => html`<${PresubmitRow} key=${job.id} job=${job} slots=${data.sparkline_slots} window=${window} />`;
  }
  jobs = jobs.filter(job => selected.has(jobRelease(job)) && (!platforms.length || platforms.some(platform => (job.platforms || []).includes(platform))));
  const grouped = groupJobs(jobs, group, releasesInOrder(snapshot.releases));
  return html`<${Fragment}>
    ${view === "presubmit" && html`<${AlertBanner} alerts=${data.alerts} />`}
    <table>
      <thead><tr><th>${title}</th><th>Pass Rate</th><th>${WINDOWS[window].label}</th><th class="job-links-header">Links</th></tr></thead>
      <tbody>
        ${!jobs.length && html`<tr><td colspan="4" class="loading">${empty}</td></tr>`}
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
