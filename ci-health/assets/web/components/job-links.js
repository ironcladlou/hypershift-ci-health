import { html, sippyJobURL } from "../ui.js";

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

export function JobLinks({ job, release, presubmit = false, registry }) {
  const name = job.prow || job.name;
  const enabled = !presubmit || job.sippy_ingestion_enabled;
  const sippy = job.sippy_url || (job.prow && enabled ? sippyJobURL(job.prow, release) : "");
  return html`<td class="job-links">
    ${sippy && html`<a class="job-link-icon" href=${sippy} target="_blank" rel="noopener" title="Sippy analysis" aria-label=${`Sippy analysis for ${name}`}><${SippyIcon} /></a>`}
    ${job.prow_job_history_url && html`<a class="job-link-icon" href=${job.prow_job_history_url} target="_blank" rel="noopener" title="Prow job history" aria-label=${`Prow history for ${name}`}><${ProwIcon} /></a>`}
    ${(job.id || job.prow) && registry && html`<button type="button" class="job-link-icon registry-link-icon" title="Registry details" aria-label=${`Registry details for ${name}`}
      aria-expanded=${registry.expanded} aria-controls=${registry.panelID} onClick=${registry.toggle}>i</button>`}
  </td>`;
}

