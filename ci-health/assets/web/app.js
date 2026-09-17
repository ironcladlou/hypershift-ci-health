import { render } from "preact";
import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { Header, Navigation } from "./components/controls.js";
import { HealthView } from "./components/health.js";
import { RegistryView } from "./components/registry.js";
import { formatAge, html, releasesInOrder, WINDOWS } from "./ui.js";

const VIEWS = new Set(["presubmit", "payload", "component", "registry"]);
const GROUPS = new Set(["", "platform", "release", "platform-release", "release-platform"]);
const DEFAULT_WINDOW = "2w";
const DEFAULT_GROUP = "release-platform";
const PATHS = {
  presubmit: "/presubmits",
  payload: "/payload",
  component: "/component-readiness",
  registry: "/registry",
};
const VIEWS_BY_PATH = Object.fromEntries(Object.entries(PATHS).map(([view, path]) => [path, view]));

function stateFromURL() {
  const query = new URLSearchParams(location.search);
  const window = query.get("window") || DEFAULT_WINDOW;
  const view = VIEWS_BY_PATH[location.pathname] || "presubmit";
  const group = query.has("group") ? query.get("group") : DEFAULT_GROUP;
  return {
    window: WINDOWS[window] ? window : DEFAULT_WINDOW,
    view: VIEWS.has(view) ? view : "presubmit",
    group: GROUPS.has(group) ? group : DEFAULT_GROUP,
    platforms: [...new Set(query.getAll("platform").filter(Boolean))],
    releaseStart: query.get("release-newest") || "",
    releaseEnd: query.get("release-oldest") || "",
    search: query.get("q") || "",
  };
}

function stateURL(state) {
  const query = new URLSearchParams();
  if (state.view === "registry") {
    if (state.search) query.set("q", state.search);
    return `${PATHS.registry}${query.size ? `?${query}` : ""}`;
  }
  if (state.window !== DEFAULT_WINDOW) query.set("window", state.window);
  if (state.group !== DEFAULT_GROUP) query.set("group", state.group);
  for (const platform of state.platforms) query.append("platform", platform);
  if (state.releaseStart) query.set("release-newest", state.releaseStart);
  if (state.releaseEnd) query.set("release-oldest", state.releaseEnd);
  return `${PATHS[state.view]}${query.size ? `?${query}` : ""}`;
}

function App() {
  const [state, setState] = useState(stateFromURL);
  const [snapshots, setSnapshots] = useState({});
  const [registry, setRegistry] = useState(null);
  const [registryError, setRegistryError] = useState("");
  const [healthError, setHealthError] = useState("");
  const [displayedWindow, setDisplayedWindow] = useState(null);
  const [loadingWindow, setLoadingWindow] = useState(null);
  const snapshotsRef = useRef({});
  const requestsRef = useRef(new Map());
  const requestedWindowRef = useRef(state.window);
  const snapshot = displayedWindow ? snapshots[displayedWindow] : null;
  const releases = useMemo(() => releasesInOrder(snapshot?.releases), [snapshot]);

  const fetchHealthWindow = useCallback((window, force = false) => {
    if (!force && snapshotsRef.current[window]) return Promise.resolve(snapshotsRef.current[window]);
    if (requestsRef.current.has(window)) return requestsRef.current.get(window);
    const request = fetch(`/_dashboard/health/windows/${encodeURIComponent(window)}`).then(async response => {
      if (!response.ok) throw new Error(response.status === 503 ? "Waiting for data…" : `${response.status} ${response.statusText}`);
      const value = await response.json();
      snapshotsRef.current = { ...snapshotsRef.current, [window]: value };
      setSnapshots(snapshotsRef.current);
      return value;
    }).finally(() => requestsRef.current.delete(window));
    requestsRef.current.set(window, request);
    return request;
  }, []);

  const presentHealthWindow = useCallback(async (window, force = false) => {
    setLoadingWindow(window);
    setHealthError("");
    try {
      await fetchHealthWindow(window, force);
      if (requestedWindowRef.current === window) setDisplayedWindow(window);
    } catch (cause) {
      if (requestedWindowRef.current === window) setHealthError(cause.message);
    } finally {
      setLoadingWindow(current => current === window ? null : current);
    }
  }, [fetchHealthWindow]);

  useEffect(() => {
    requestedWindowRef.current = state.window;
    const cached = snapshotsRef.current[state.window];
    if (cached) {
      setHealthError("");
      setLoadingWindow(null);
      setDisplayedWindow(state.window);
      return;
    }
    presentHealthWindow(state.window);
  }, [presentHealthWindow, state.window]);
  useEffect(() => {
    const timer = setInterval(() => presentHealthWindow(requestedWindowRef.current, true), 15 * 60 * 1000);
    return () => clearInterval(timer);
  }, [presentHealthWindow]);
  useEffect(() => {
    if (!displayedWindow) return;
    for (const window of Object.keys(WINDOWS)) {
      if (window !== displayedWindow) fetchHealthWindow(window).catch(() => {});
    }
  }, [displayedWindow, fetchHealthWindow]);
  useEffect(() => {
    if (state.view !== "registry" || registry) return;
    fetch("/api/job-registry").then(response => {
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      return response.json();
    }).then(setRegistry).catch(cause => setRegistryError(cause.message));
  }, [registry, state.view]);

  useEffect(() => {
    if (!releases.length) return;
    const startIndex = releases.indexOf(state.releaseStart);
    const endIndex = releases.indexOf(state.releaseEnd);
    if (startIndex >= 0 && endIndex >= startIndex) return;
    setState(current => ({ ...current, releaseStart: releases[0], releaseEnd: releases[Math.min(2, releases.length - 1)] }));
  }, [releases, state.releaseEnd, state.releaseStart]);

  useEffect(() => {
    if (state.platforms.length && snapshot) {
      const available = new Set(snapshot.platforms || []);
      const platforms = state.platforms.filter(platform => available.has(platform));
      if (platforms.length !== state.platforms.length) {
        setState(current => ({ ...current, platforms }));
        return;
      }
    }
    const url = stateURL(state);
    if (`${location.pathname}${location.search}` !== url) history.replaceState(null, "", url);
  }, [snapshot, state]);

  useEffect(() => {
    const restore = () => setState(stateFromURL());
    addEventListener("popstate", restore);
    return () => removeEventListener("popstate", restore);
  }, []);

  const update = (values, push = false) => {
    const next = { ...state, ...values };
    if (push) history.pushState(null, "", stateURL(next));
    setState(next);
  };
  const setRange = (start, end) => update({ releaseStart: releases[start], releaseEnd: releases[end] });
  const selectedReleases = releases.slice(Math.max(0, releases.indexOf(state.releaseStart)), Math.max(0, releases.indexOf(state.releaseEnd)) + 1);
  const failedCollections = snapshot?.collection?.filter(item => item.error).length || 0;
  const status = healthError
    ? { text: healthError, className: "cache-error" }
    : snapshot?.complete === false
      ? { text: `Partial report · ${failedCollections} failed request${failedCollections === 1 ? "" : "s"}`, className: "cache-error" }
      : snapshot
        ? { text: `Updated ${formatAge(snapshot.generated_at)}`, className: "cache-fresh" }
        : { text: "Loading…", className: "cache-stale" };

  return html`<div class="page">
    <${Header} state=${state} platforms=${snapshot?.platforms || []} status=${status} refreshing=${loadingWindow !== null}
      onWindow=${window => update({ window }, true)} onGroup=${group => update({ group })}
      onPlatforms=${platforms => update({ platforms })} onRefresh=${() => presentHealthWindow(state.window, true)} />
    <${Navigation} view=${state.view} onView=${view => update({ view }, true)} releases=${releases}
      start=${state.releaseStart} end=${state.releaseEnd} onRange=${setRange} />
    ${state.view === "registry"
      ? registry
        ? html`<${RegistryView} registry=${registry} query=${state.search} onQuery=${search => update({ search })} />`
        : html`<div class=${`loading ${registryError ? "error" : ""}`}>${registryError ? `Failed to load job registry: ${registryError}` : "Loading job registry…"}</div>`
      : snapshot
        ? html`<${HealthView} snapshot=${snapshot} view=${state.view} group=${state.group} platforms=${state.platforms}
            selectedReleases=${selectedReleases} window=${displayedWindow} />`
        : html`<div class=${`loading ${healthError ? "error" : ""}`}>${healthError || "Loading health window…"}</div>`}
  </div>`;
}

render(html`<${App} />`, document.getElementById("app"));
