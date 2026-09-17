import { html } from "../ui.js";

function Toggle({ values, active, onChange, label }) {
  return html`<div class="toggle" role="group" aria-label=${label}>
    ${values.map(item => html`<button key=${item.value} class=${active === item.value ? "active" : ""} onClick=${() => onChange(item.value)}>${item.label}</button>`)}
  </div>`;
}

const GROUPS = [
  { value: "", label: "None" },
  { value: "platform", label: "Platform" },
  { value: "release", label: "Release" },
  { value: "platform-release", label: "Platform → Release" },
  { value: "release-platform", label: "Release → Platform" },
];

function PlatformFilter({ platforms, selected, onChange }) {
  const enabled = new Set(selected);
  const summary = selected.length === 0 ? "All" : selected.length === 1 ? selected[0] : `${selected.length} selected`;
  const toggle = platform => onChange(enabled.has(platform)
    ? selected.filter(value => value !== platform)
    : [...selected, platform]);
  return html`<details class="header-filter platform-filter" name="header-filter">
    <summary aria-label="Platform filter">Platforms <span>${summary}</span></summary>
    <div class="filter-menu platform-menu">
      <div class="filter-menu-header"><strong>Platforms</strong><button type="button" disabled=${selected.length === 0} onClick=${() => onChange([])}>Clear</button></div>
      ${platforms.map(platform => html`<label key=${platform}>
        <input type="checkbox" value=${platform} checked=${enabled.has(platform)} onChange=${() => toggle(platform)} />
        <span>${platform}</span>
      </label>`)}
    </div>
  </details>`;
}

function GroupFilter({ selected, onChange }) {
  const current = GROUPS.find(group => group.value === selected) || GROUPS[0];
  const choose = (event, value) => {
    event.currentTarget.closest("details").open = false;
    onChange(value);
  };
  return html`<details class="header-filter group-filter" name="header-filter">
    <summary aria-label="Grouping">Group by <span>${current.label}</span></summary>
    <div class="filter-menu group-menu">
      ${GROUPS.map(group => html`<label key=${group.value}>
        <input type="radio" name="grouping" value=${group.value} checked=${group.value === selected} onChange=${event => choose(event, group.value)} />
        <span>${group.label}</span>
      </label>`)}
    </div>
  </details>`;
}

export function Header({ state, platforms, status, refreshing, onWindow, onGroup, onPlatforms, onRefresh }) {
  const registry = state.view === "registry";
  return html`<header class="header">
    <div>
      <h1>HyperShift CI Health</h1>
      <nav class="header-links" aria-label="Project links">
        <a href="https://github.com/ironcladlou/hypershift-ci-health" target="_blank" rel="noopener">Source code</a>
        <span aria-hidden="true">·</span><a href="/api/docs" target="_blank" rel="noopener">Job Registry API docs</a>
      </nav>
    </div>
    <div class="header-meta">
      ${!registry && html`<${PlatformFilter} platforms=${platforms} selected=${state.platforms} onChange=${onPlatforms} />`}
      ${!registry && html`<${GroupFilter} selected=${state.group} onChange=${onGroup} />`}
      ${!registry && html`<${Toggle} label="Health window" active=${state.window} onChange=${onWindow} values=${[
        { value: "1w", label: "1w" }, { value: "2w", label: "2w" }, { value: "1m", label: "1m" },
      ]} />`}
      <span class=${status.className}>${status.text}</span>
      <button class=${`refresh-btn ${refreshing ? "spinning" : ""}`} onClick=${onRefresh} aria-label="Refresh">↻</button>
    </div>
  </header>`;
}

export function Navigation({ view, onView, releases, start, end, onRange }) {
  const min = Math.max(0, releases.indexOf(start));
  const endIndex = releases.indexOf(end);
  const max = endIndex >= min ? endIndex : Math.min(2, releases.length - 1);
  const denominator = Math.max(1, releases.length - 1);
  const first = releases[min];
  const last = releases[max];
  const rangeLabel = first === last ? first : `${first}–${last} · ${max - min + 1} releases`;
  const setBound = (bound, value) => {
    const index = Number(value);
    onRange(bound === "start" ? Math.min(index, max) : min, bound === "end" ? Math.max(index, min) : max);
  };
  return html`<div class="view-navigation">
    <${Toggle} label="View" active=${view} onChange=${onView} values=${[
      { value: "presubmit", label: "Presubmit" }, { value: "payload", label: "Release Payload" },
      { value: "component", label: "Component Readiness" }, { value: "registry", label: "Job Registry" },
    ]} />
    ${view !== "registry" && releases.length > 0 && html`<div class="release-filter">
      <span class="release-filter-label">Releases <strong>${rangeLabel}</strong></span>
      <div class=${`release-range-control ${min === max ? "collapsed" : ""}`}>
        <div class="release-range-track"><span style=${{ left: `${100 * min / denominator}%`, width: `${100 * (max - min) / denominator}%` }}></span></div>
        <input class="range-start" type="range" min="0" max=${releases.length - 1} value=${min} disabled=${releases.length === 1} aria-label="Newest release" onInput=${event => setBound("start", event.currentTarget.value)} />
        <input class="range-end" type="range" min="0" max=${releases.length - 1} value=${max} disabled=${releases.length === 1} aria-label="Oldest release" onInput=${event => setBound("end", event.currentTarget.value)} />
        <div class="release-range-ticks" aria-hidden="true">${releases.map((release, index) => html`<span key=${release} class=${index >= min && index <= max ? "selected" : ""}>${release}</span>`)}</div>
      </div>
    </div>`}
  </div>`;
}
