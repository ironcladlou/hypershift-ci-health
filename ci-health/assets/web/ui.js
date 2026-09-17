import { Fragment, h } from "preact";
import htm from "htm";

export const html = htm.bind(h);
export { Fragment };

export const SIPPY = "https://sippy.dptools.openshift.org";

export const WINDOWS = {
  "1w": { label: "Last 1w", dateEvery: 1 },
  "2w": { label: "Last 2w", dateEvery: 2 },
  "1m": { label: "Last 1m", dateEvery: 5 },
};

export function sippyJobURL(jobName, release) {
  const filters = JSON.stringify({
    items: [{ columnField: "name", operatorValue: "equals", value: jobName }],
  });
  return `${SIPPY}/sippy-ng/jobs/${encodeURIComponent(release)}/analysis?filters=${encodeURIComponent(filters)}`;
}

export function releaseRank(release) {
  const [major, minor] = String(release).split(".").map(Number);
  if (!Number.isInteger(major) || !Number.isInteger(minor)) return -1;
  if (major === 4) return minor;
  if (major >= 5) return 23 + (major - 5) * 100 + minor;
  return -1;
}

export function releasesInOrder(releases) {
  return [...new Set(releases || [])].sort((a, b) => releaseRank(b) - releaseRank(a));
}

export function jobRelease(job) {
  if (job.target_release) return job.target_release;
  return job.periodics?.[0]?.release || job.release || "";
}

export function platformLabel(job) {
  return (job.platforms || []).join(" / ") || "unknown";
}

export function rateClass(rate, runs) {
  if (!runs) return "rate-na";
  if (rate >= 80) return "rate-good";
  if (rate >= 60) return "rate-warn";
  return "rate-bad";
}

export function statusClass(rate, runs) {
  if (!runs) return "status-na";
  if (rate >= 80) return "status-good";
  if (rate >= 60) return "status-warn";
  return "status-bad";
}

export function formatAge(timestamp) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(timestamp).getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m ago`;
}

export function groupJobs(jobs, groupBy, releases) {
  const sorted = values => [...values].sort((a, b) =>
    (a.name || "").localeCompare(b.name || "") || (a.prow || "").localeCompare(b.prow || ""));
  if (!groupBy) return [{ jobs: sorted(jobs) }];

  const dimensions = groupBy.split("-");
  const value = (job, dimension) => dimension === "platform" ? platformLabel(job) : jobRelease(job) || "unknown";
  const label = (item, dimension) => {
    if (dimension !== "release") return item;
    const index = releases.indexOf(item);
    if (index === 0) return `Future (${item})`;
    if (index > 0) return `N-${index} (${item})`;
    return item || "unknown";
  };
  const order = (values, dimension) => [...values].sort((a, b) =>
    dimension === "release" ? releases.indexOf(a) - releases.indexOf(b) : a.localeCompare(b));
  const partition = (values, dimension) => {
    const groups = new Map();
    for (const job of values) {
      const key = value(job, dimension);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(job);
    }
    return groups;
  };

  const outer = dimensions[0];
  const result = [];
  const groups = partition(jobs, outer);
  for (const key of order(groups.keys(), outer)) {
    result.push({ heading: label(key, outer), dimension: outer });
    if (dimensions.length === 1) {
      result.push({ jobs: sorted(groups.get(key)) });
      continue;
    }
    const inner = dimensions[1];
    const subgroups = partition(groups.get(key), inner);
    for (const innerKey of order(subgroups.keys(), inner)) {
      result.push({ subheading: label(innerKey, inner), dimension: inner });
      result.push({ jobs: sorted(subgroups.get(innerKey)) });
    }
  }
  return result;
}
