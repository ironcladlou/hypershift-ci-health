import { useEffect, useMemo, useState } from "preact/hooks";
import { html } from "../ui.js";

const PAGE_SIZE = 100;
const VALUE_LABELS = {
  "in-horizon": "Within release horizon", upcoming: "Upcoming", unknown: "Unknown",
  development: "Development branch", release: "Release branch", prospective: "Prospective release branch",
  periodic: "Periodic", presubmit: "Presubmit", none: "Non-E2E", v1: "E2E v1", v2: "E2E v2",
  required: "Required", optional: "Optional", "always-run": "Always run", "sippy-enabled": "Sippy enabled",
  "sippy-disabled": "Sippy disabled", "has-counterpart": "Has counterpart", blocking: "Blocking",
  informing: "Informing", async: "Async", disabled: "Disabled", presubmit_health: "Presubmit Health",
  release_payload: "Release Payload", component_readiness: "Component Readiness", not_selected: "Not selected",
  "human-verified": "Human verified",
};

export function label(value) {
  return VALUE_LABELS[value] || value;
}

export function Tag({ children, tone = "" }) {
  return html`<span class=${`registry-tag ${tone}`}>${children}</span>`;
}

export function Tags({ values, empty = "—" }) {
  if (!values?.length) return empty;
  return html`<span class="registry-tags">${values.map(value => html`<${Tag} key=${value}>${label(value)}<//>`)}</span>`;
}

function Facet({ name, values, selected, onChange, facetLabels }) {
  const summary = selected.length ? `${facetLabels[name]} (${selected.length})` : facetLabels[name];
  return html`<details class="registry-facet" name="registry-facet">
    <summary>${summary}</summary>
    <div class="registry-facet-menu">
      ${values.map(item => html`<label key=${item.value}>
        <input type="checkbox" checked=${selected.includes(item.value)} onChange=${() => onChange(item.value)} />
        <span>${label(item.value)}</span><small>${item.count}</small>
      </label>`)}
    </div>
  </details>`;
}

export function SortButton({ value, active, order, onSort, children }) {
  return html`<button class=${active === value ? "active" : ""} onClick=${() => onSort(value)}>
    ${children}${active === value ? order === "asc" ? " ↑" : " ↓" : ""}
  </button>`;
}

export function Pagination({ page, pages, total, onPage }) {
  if (!total) return null;
  const start = (page - 1) * PAGE_SIZE + 1;
  const end = Math.min(total, page * PAGE_SIZE);
  return html`<nav class="registry-pagination" aria-label="Job result pages">
    <span>${start}–${end} of ${total}</span>
    <button disabled=${page === 1} onClick=${() => onPage(page - 1)}>Previous</button>
    <span>Page ${page} of ${pages}</span>
    <button disabled=${page === pages} onClick=${() => onPage(page + 1)}>Next</button>
  </nav>`;
}

export function BrowserToolbar({ query, total, count, facetNames, facetLabels, options, filters, onState, setFilter, clearFilters, searchLabel }) {
  const activeFilters = facetNames.flatMap(name => (filters[name] || []).map(value => ({ name, value })));
  return html`<div class="registry-toolbar">
    <div class="registry-search-row">
      <input type="search" value=${query} placeholder="Search jobs and configuration…" aria-label=${searchLabel}
        onInput=${event => onState({ search: event.currentTarget.value, registryPage: 1, selectedJob: "" })} />
      <span>${count} of ${total} jobs</span>
    </div>
    <div class="registry-facets" aria-label="Job filters">
      ${facetNames.map(name => html`<${Facet} key=${name} name=${name} facetLabels=${facetLabels} values=${options[name]}
        selected=${filters[name] || []} onChange=${value => setFilter(name, value)} />`)}
    </div>
    ${activeFilters.length ? html`<div class="registry-active-filters">
      ${activeFilters.map(item => html`<button key=${`${item.name}-${item.value}`} onClick=${() => setFilter(item.name, item.value)}>${facetLabels[item.name]}: ${label(item.value)} ×</button>`)}
      <button class="registry-clear" onClick=${clearFilters}>Clear all</button>
    </div>` : null}
  </div>`;
}

// Filtering, facet counts, search, pagination, and selection are shared; each
// view supplies its own comparator, facets, records, and rendering.
export function useJobBrowser({ jobs, facetValues, searchable, facetNames, query, filters, sort, order, page, selectedJob, onState, compare, promoteExact = false }) {
  const [Fuse, setFuse] = useState(null);
  useEffect(() => {
    let current = true;
    import("/assets/vendor/fuse/fuse.min.mjs").then(module => current && setFuse(() => module.default));
    return () => { current = false; };
  }, []);
  const fuse = useMemo(() => Fuse ? new Fuse(searchable, { threshold: 0.4, ignoreLocation: true,
    keys: Object.keys(searchable[0] || {}).filter(key => key !== "job") }) : null, [Fuse, searchable]);
  const options = useMemo(() => Object.fromEntries(facetNames.map(name => {
    const counts = new Map();
    for (const job of jobs) for (const value of facetValues.get(job.id)[name] || []) counts.set(value, (counts.get(value) || 0) + 1);
    return [name, [...counts].map(([value, count]) => ({ value, count })).sort((a, b) => label(a.value).localeCompare(label(b.value)))];
  })), [facetValues, jobs, facetNames]);
  const visible = useMemo(() => {
    const value = query.trim();
    const exact = value.toLocaleLowerCase();
    let result = value
      ? fuse ? fuse.search(value).map(match => match.item.job)
        : searchable.filter(item => Object.values(item).some(field => typeof field === "string" && field.toLowerCase().includes(value.toLowerCase()))).map(item => item.job)
      : [...jobs];
    result = result.filter(job => facetNames.every(name => !filters[name]?.length || filters[name].some(selected => (facetValues.get(job.id)[name] || []).includes(selected))));
    result.sort((left, right) => {
      if (promoteExact) {
        const leftExact = exact && [left.name, left.id].some(candidate => candidate?.toLocaleLowerCase() === exact);
        const rightExact = exact && [right.name, right.id].some(candidate => candidate?.toLocaleLowerCase() === exact);
        if (leftExact !== rightExact) return leftExact ? -1 : 1;
      }
      return compare(left, right) * (order === "desc" ? -1 : 1);
    });
    return result;
  }, [facetValues, filters, fuse, jobs, order, query, searchable, sort, compare, promoteExact, facetNames]);
  const pages = Math.max(1, Math.ceil(visible.length / PAGE_SIZE));
  const currentPage = Math.min(page, pages);
  const selectedIndex = visible.findIndex(job => job.id === selectedJob);
  useEffect(() => {
    if (page > pages) onState({ registryPage: pages });
    else if (selectedIndex >= 0 && Math.floor(selectedIndex / PAGE_SIZE) + 1 !== page) onState({ registryPage: Math.floor(selectedIndex / PAGE_SIZE) + 1 });
  }, [onState, page, pages, selectedIndex]);
  useEffect(() => {
    if (!selectedJob) return;
    requestAnimationFrame(() => {
      const row = document.getElementById(`registry-job-${selectedJob}`);
      row?.scrollIntoView({ block: "center" });
      row?.querySelector(".registry-job-link")?.focus({ preventScroll: true });
    });
  }, [currentPage, selectedJob]);
  const setFilter = (name, value) => onState({ registryFilters: { ...filters, [name]: (filters[name] || []).includes(value)
    ? filters[name].filter(item => item !== value) : [...(filters[name] || []), value] }, registryPage: 1, selectedJob: "" });
  const clearFilters = () => onState({ registryFilters: Object.fromEntries(facetNames.map(name => [name, []])), registryPage: 1, selectedJob: "" });
  const resetAll = () => onState({ search: "", registryFilters: Object.fromEntries(facetNames.map(name => [name, []])), registryPage: 1, selectedJob: "" });
  return { pageJobs: visible.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE), visible, pages, currentPage, options, setFilter, clearFilters, resetAll };
}
