import { useEffect, useMemo, useState } from "preact/hooks";
import { Fragment, html } from "../ui.js";
import { Tooltip } from "./tooltip.js";

function RegistryEntry({ job }) {
  const [expanded, setExpanded] = useState(false);
  const [detail, setDetail] = useState(null);
  const [error, setError] = useState("");
  const entryURL = `/api/job-registry/jobs/${encodeURIComponent(job.id)}`;
  const participation = (job.release_controller || []).map(item => {
    const stream = item.stream || {};
    const verification = item.verification || {};
    return { label: `${stream.release || stream.name || "unknown"} · ${verification.role || "unknown"}`, url: stream.release_status_url };
  });
  const ingestion = job.presubmit?.sippy_ingestion;

  async function toggle(event) {
    event.preventDefault();
    const opening = !expanded;
    setExpanded(opening);
    if (!opening || detail || error) return;
    try {
      const response = await fetch(entryURL, { headers: { Accept: "application/json" } });
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      setDetail(await response.json());
    } catch (cause) {
      setError(cause.message);
    }
  }

  return html`<${Fragment}>
    <tr class="registry-job">
      <td class="job-name">
        ${job.prow_job_history_url || job.source?.url
          ? html`<a href=${job.prow_job_history_url || job.source.url} target="_blank" rel="noopener">${job.name}</a>`
          : job.name}
        <br /><span class="registry-detail">
          ${ingestion && !ingestion.enabled
            ? html`<${Tooltip} content=${ingestion.basis}>Sippy ingestion disabled<//>`
            : job.sippy_url
              ? html`<a href=${job.sippy_url} target="_blank" rel="noopener">Sippy</a>`
              : html`<span>Sippy unavailable</span>`}
          ${job.source?.url && html` · <a href=${job.source.url} target="_blank" rel="noopener">source</a>`}
          · <a href=${entryURL} aria-expanded=${expanded} onClick=${toggle}>${expanded ? "Hide API JSON" : "API JSON"}</a>
          · <a href=${entryURL} target="_blank" rel="noopener">raw API</a>
          · <a href="/api/schemas/Job.json" target="_blank" rel="noopener">schema</a>
        </span>
      </td>
      <td><span class="job-type no-margin">${job.type}</span></td>
      <td>${job.repository || "—"}<br /><span class="registry-detail">${job.context || "—"}</span></td>
      <td>${(job.versions || []).join(", ") || "—"}<br /><span class="registry-detail">${(job.platforms || []).join(", ") || "—"}</span></td>
      <td>${participation.length ? participation.map((item, index) => html`<div key=${index}>${item.url
        ? html`<a href=${item.url} target="_blank" rel="noopener">${item.label}</a>` : item.label}</div>`) : "—"}</td>
    </tr>
    ${expanded && html`<tr class="registry-json-row"><td class="registry-json-cell" colspan="5"><div class="registry-json-panel"><pre>${error ? `Failed to load registry entry: ${error}` : detail ? JSON.stringify(detail, null, 2) : "Loading registry entry…"}</pre></div></td></tr>`}
  <//>`;
}

export function RegistryView({ registry, query, onQuery }) {
  const [Fuse, setFuse] = useState(null);
  useEffect(() => {
    let current = true;
    import("/assets/vendor/fuse/fuse.min.mjs").then(module => current && setFuse(() => module.default));
    return () => { current = false; };
  }, []);
  const jobs = useMemo(() => [...(registry.jobs || [])].sort((a, b) => (a.name || "").localeCompare(b.name || "")), [registry]);
  const searchable = useMemo(() => jobs.map(job => ({
    job, id: job.id, name: job.name || "", type: job.type || "", repository: job.repository || "",
    context: job.context || "", versions: (job.versions || []).join(" "), platforms: (job.platforms || []).join(" "),
    releases: (job.release_controller || []).flatMap(item => [item.stream?.name, item.stream?.release, item.verification?.role]).filter(Boolean).join(" "),
  })), [jobs]);
  const visible = useMemo(() => {
    const value = query.trim();
    if (!value) return jobs;
    if (Fuse) {
      const fuse = new Fuse(searchable, { threshold: 0.4, ignoreLocation: true, useTokenSearch: true,
        keys: ["name", "id", "type", "repository", "context", "versions", "platforms", "releases"] });
      return fuse.search(value).map(result => result.item.job);
    }
    const lower = value.toLowerCase();
    return searchable.filter(item => Object.values(item).some(field => typeof field === "string" && field.toLowerCase().includes(lower))).map(item => item.job);
  }, [Fuse, jobs, query, searchable]);

  return html`<${Fragment}>
    <div class="registry-toolbar">
      <input type="search" value=${query} placeholder="Search jobs (fuzzy)…" aria-label="Fuzzy search job registry" onInput=${event => onQuery(event.currentTarget.value)} />
      <span>${visible.length} ${visible.length === 1 ? "job" : "jobs"}</span>
    </div>
    <table class="registry-table">
      <thead><tr><th>Job</th><th>Type</th><th>Repository / context</th><th>Versions / platforms</th><th>Release participation</th></tr></thead>
      <tbody>${visible.map(job => html`<${RegistryEntry} key=${job.id} job=${job} />`)}</tbody>
    </table>
  <//>`;
}
