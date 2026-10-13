import { expect, test } from "@playwright/test";

test("policy browser preserves global ranks, filters, and shareable selection without health requests", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  const healthRequests = [];
  const errors = [];
  page.on("request", request => { if (request.url().includes("/_dashboard/health/")) healthRequests.push(request.url()); });
  page.on("pageerror", error => errors.push(error.message));
  await page.goto("/policy");
  await expect(page.getByRole("heading", { name: "Job Policy · hypershift" })).toBeVisible();
  await expect(page.getByRole("group", { name: "Health window" })).toHaveCount(0);
  await expect(page.locator(".policy-table tr.registry-job")).toHaveCount(Math.min(100, catalog.jobs.length));
  await expect(page.locator(".policy-rank").first()).toHaveText("1");
  await expect(page.locator(".registry-job-link").first()).toHaveText(catalog.jobs[0].job.name);
  await expect(page.getByRole("columnheader", { name: "Health objective" })).toBeVisible();

  const platformFacet = page.locator(".registry-facet").filter({ has: page.locator("summary", { hasText: /^Platform/ }) });
  await platformFacet.locator("summary").click();
  await platformFacet.getByRole("checkbox", { name: /^aws/ }).check();
  const awsJobs = catalog.jobs.filter(entry => entry.job.platforms.includes("aws"));
  await expect(page.locator(".policy-table tr.registry-job")).toHaveCount(Math.min(100, awsJobs.length));
  await expect.poll(() => page.locator(".policy-rank").allTextContents()).toEqual(awsJobs.slice(0, 100).map(entry => String(entry.team_metadata.importance.rank)));
  await expect.poll(() => new URL(page.url()).searchParams.getAll("platform")).toEqual(["aws"]);

  const search = page.getByRole("searchbox", { name: "Search job policy" });
  await search.fill(awsJobs[0].job.name);
  const selected = page.locator(".registry-job-link").filter({ hasText: awsJobs[0].job.name }).first();
  await selected.click();
  await expect(page.getByRole("heading", { name: "Policy explanation" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Identity and source" })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get("job")).toBe(awsJobs[0].job.id);
  const selectedURL = page.url();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Policy explanation" })).toBeVisible();
  await expect(search).toHaveValue(awsJobs[0].job.name);
  expect(page.url()).toBe(selectedURL);
  expect(healthRequests).toEqual([]);
  expect(errors).toEqual([]);

  await page.getByRole("link", { name: "View in Job Registry" }).click();
  await expect(page.getByRole("searchbox", { name: "Search job registry" })).toBeVisible();
  await expect(page.locator("tr.registry-job.expanded .registry-job-link")).toHaveText(awsJobs[0].job.name);
});

test("policy search keeps importance ordering and selected-job links locate the correct page", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  await page.goto("/policy?q=aws");
  await expect(page.locator(".policy-rank").first()).toBeVisible();
  await expect.poll(async () => {
    const ranks = (await page.locator(".policy-rank").allTextContents()).map(Number);
    return ranks.every((rank, index) => index === 0 || rank > ranks[index-1]);
  }).toBe(true);
  const job = catalog.jobs[Math.min(110, catalog.jobs.length-1)];
  await page.goto(`/policy?job=${encodeURIComponent(job.job.id)}`);
  await expect(page.locator("tr.registry-job.expanded .policy-rank")).toHaveText(String(job.team_metadata.importance.rank));
  await expect.poll(() => new URL(page.url()).searchParams.get("page")).toBe(String(Math.floor((job.team_metadata.importance.rank-1)/100)+1));
  await page.getByRole("button", { name: /^Job(?: ↑| ↓)?$/ }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("sort")).toBe("name");
  await page.getByRole("button", { name: /^Rank(?: ↑| ↓)?$/ }).click();
  await expect(page.locator(".policy-rank").first()).toHaveText("1");
});

test("a policy API failure leaves registry browsing available", async ({ page }) => {
  await page.route("**/api/teams/hypershift/jobs", route => route.fulfill({ status: 503, contentType: "application/json", body: '{"detail":"Catalog unavailable"}' }));
  await page.goto("/policy");
  await expect(page.locator(".loading.error")).toContainText("Failed to load job policy");
  await page.getByRole("button", { name: "Job Registry", exact: true }).click();
  await expect(page.getByRole("searchbox", { name: "Search job registry" })).toBeVisible();
  await expect(page.locator(".registry-job")).not.toHaveCount(0);
});

test("non-E2E presubmits rank first within their branch scopes and explain framework priority", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  for (const group of ["required-auto-run", "other"]) {
    for (const scope of ["development", "release"]) {
      const presubmits = catalog.jobs.filter(entry => entry.job.type === "presubmit" &&
        entry.team_metadata.importance.group === group && entry.team_metadata.importance.presubmit_branch_scope === scope && entry.team_metadata.importance.release_scope !== "upcoming");
      const nonE2E = presubmits.filter(entry => entry.job.e2e_framework === "none");
      const e2e = presubmits.filter(entry => ["v1", "v2"].includes(entry.job.e2e_framework));
      if (!nonE2E.length || !e2e.length) continue;
      expect(Math.max(...nonE2E.map(entry => entry.team_metadata.importance.rank)))
        .toBeLessThan(Math.min(...e2e.map(entry => entry.team_metadata.importance.rank)));
    }
  }
  const job = catalog.jobs.find(entry => entry.job.id === "pull-ci-openshift-hypershift-main-images");
  await page.goto(`/policy?job=${encodeURIComponent(job.job.id)}`);
  await expect(page.locator("tr.registry-job.expanded .policy-rank")).toHaveText(String(job.team_metadata.importance.rank));
  await expect(page.locator(".policy-explanation")).toContainText("Framework priority");
  await expect(page.locator(".policy-explanation")).toContainText("Non-E2E presubmit; preferred");
});

test("development presubmits outrank prospective release definitions and explain branch applicability", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  const mainID = "pull-ci-openshift-hypershift-main-images";
  const releaseID = "pull-ci-openshift-hypershift-release-5.1-images";
  const main = catalog.jobs.find(entry => entry.job.id === mainID);
  const release = catalog.jobs.find(entry => entry.job.id === releaseID);
  expect(main.team_metadata.importance.rank).toBeLessThan(release.team_metadata.importance.rank);
  expect(main.team_metadata.importance.resolved_release).toBe(catalog.policy.development_release);
  expect(main.team_metadata.importance.presubmit_branch_scope).toBe("development");
  expect(release.team_metadata.importance.presubmit_branch_scope).toBe("prospective");
  const active = catalog.jobs.filter(entry => entry.team_metadata.importance.release_scope !== "upcoming" && entry.team_metadata.importance.presubmit_branch_scope !== "prospective");
  expect(Math.max(...active.map(entry => entry.team_metadata.importance.rank))).toBeLessThan(release.team_metadata.importance.rank);
  await page.goto(`/policy?job=${encodeURIComponent(releaseID)}`);
  await expect(page.locator("tr.registry-job.expanded")).toContainText("Prospective release branch");
  await expect(page.locator(".policy-explanation")).toContainText("It ranks after active jobs");
  await expect(page.locator(".policy-explanation")).toContainText("required and always-run flags apply to matching target branches");
  await page.goto(`/policy?job=${encodeURIComponent(mainID)}`);
  await expect(page.locator("tr.registry-job.expanded")).toContainText("Development branch");
  await expect(page.locator(".policy-explanation")).toContainText("presubmit target release");
  await page.goto("/policy?presubmit-branch=prospective");
  const prospective = catalog.jobs.filter(entry => entry.team_metadata.importance.presubmit_branch_scope === "prospective");
  await expect(page.locator(".policy-table tr.registry-job")).toHaveCount(Math.min(100, prospective.length));
  await page.reload();
  await expect(page.locator(".policy-table tr.registry-job")).toHaveCount(Math.min(100, prospective.length));
  await expect.poll(() => new URL(page.url()).searchParams.get("presubmit-branch")).toBe("prospective");
});

test("upcoming jobs stay visible after the release horizon and explain their scope", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  const upcoming = catalog.jobs.filter(entry => entry.team_metadata.importance.release_scope === "upcoming");
  expect(upcoming.length).toBeGreaterThan(0);
  const firstUpcoming = catalog.jobs.findIndex(entry => entry.team_metadata.importance.release_scope === "upcoming");
  expect(catalog.jobs.slice(firstUpcoming).every(entry => entry.team_metadata.importance.release_scope === "upcoming")).toBe(true);
  await page.goto("/policy?release-scope=upcoming");
  await expect(page.locator(".policy-context")).toContainText(`Development release ${catalog.policy.development_release}`);
  await expect(page.locator(".policy-table tr.registry-job")).toHaveCount(Math.min(100, upcoming.length));
  await expect(page.locator(".policy-rank").first()).toHaveText(String(upcoming[0].team_metadata.importance.rank));
  await page.locator(".registry-job-link").first().click();
  await expect(page.locator(".policy-explanation")).toContainText(`compared with development release ${catalog.policy.development_release}`);
  await expect(page.locator(".policy-explanation")).toContainText("Upcoming jobs follow all other jobs");
});

test("periodics linked to required presubmits rank first within groups and show their evidence", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  let comparisons = 0;
  for (const group of catalog.policy.groups) {
    const periodics = catalog.jobs.filter(entry => entry.job.type === "periodic" &&
      entry.team_metadata.importance.group === group.id && entry.team_metadata.importance.release_scope !== "upcoming");
    const required = periodics.filter(entry => entry.team_metadata.importance.required_presubmit_ids?.length);
    const remaining = periodics.filter(entry => !entry.team_metadata.importance.required_presubmit_ids?.length);
    if (!required.length || !remaining.length) continue;
    comparisons++;
    expect(Math.max(...required.map(entry => entry.team_metadata.importance.rank)))
      .toBeLessThan(Math.min(...remaining.map(entry => entry.team_metadata.importance.rank)));
  }
  expect(comparisons).toBeGreaterThan(0);
  const id = "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn";
  const parent = "pull-ci-openshift-hypershift-main-e2e-aws";
  const entry = catalog.jobs.find(entry => entry.job.id === id);
  expect(entry.team_metadata.importance.required_presubmit_ids).toContain(parent);
  await page.goto(`/policy?job=${encodeURIComponent(id)}`);
  await expect(page.locator(".policy-explanation")).toContainText("Required presubmit counterparts");
  await page.locator(".policy-explanation").getByRole("link", { name: parent, exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.get("job")).toBe(parent);
  await expect(page.locator("tr.registry-job.expanded .registry-job-link")).toHaveText(parent);
});

test("structured objectives show systemic checks for presubmits and pass-rate checks for periodics", async ({ page }) => {
  const presubmit = "pull-ci-openshift-hypershift-main-images";
  await page.goto(`/policy?job=${encodeURIComponent(presubmit)}`);
  const row = page.locator(`tr.registry-job[id="registry-job-${presubmit}"]`);
  await expect(row).toContainText("No systemic failures");
  await expect(row).not.toContainText("80%");
  const objective = page.locator(".health-objective-details");
  await expect(objective).toContainText("presubmit-systemic-health/v1");
  await expect(objective).toContainText("Job permafailing");
  await expect(objective).toContainText("5 distinct PRs");
  await expect(objective).toContainText("Correlated failures");
  await expect(objective).toContainText("3 distinct PRs and 2 jobs");
  await expect(objective).toContainText("on the same target branch");
  await expect(objective).toContainText("unrelated to PR contents");
  await expect(objective).toContainText("Indeterminate");
  await expect(objective).toContainText("not evaluated");
  const periodic = "periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn";
  await page.goto(`/policy?job=${encodeURIComponent(periodic)}`);
  await expect(page.locator(".health-objective-details")).toContainText("job-pass-rate/v1");
  await expect(page.locator(".health-objective-details")).toContainText("80% passing over 1w");
  await expect(page.locator(".health-objective-details")).toContainText("10 completed, non-aborted runs");
  await expect(page.locator(".health-objective-details")).not.toContainText("Job permafailing");
});

test("registry and policy rows share Sippy Prow and info controls", async ({ page, request }) => {
  const catalog = await (await request.get("/api/teams/hypershift/jobs")).json();
  const job = catalog.jobs.find(entry => entry.job.id === "pull-ci-openshift-hypershift-main-e2e-aws").job;
  const requests = [];
  page.on("request", request => { if (request.url().includes("/_dashboard/health/")) requests.push(request.url()); });
  for (const view of ["policy", "registry"]) {
    await page.goto(`/${view}?job=${encodeURIComponent(job.id)}`);
    const row = page.locator(`tr.registry-job[id="registry-job-${job.id}"]`);
    const sippy = row.getByRole("link", { name: `Sippy analysis for ${job.name}`, exact: true });
    const prow = row.getByRole("link", { name: `Prow history for ${job.name}`, exact: true });
    await expect(sippy).toHaveAttribute("href", job.sippy_url);
    await expect(prow).toHaveAttribute("href", job.prow_job_history_url);
    for (const link of [sippy, prow]) {
      await expect(link).toHaveAttribute("target", "_blank");
      await expect(link.locator("svg")).toBeVisible();
    }
    const info = row.getByRole("button", { name: `Registry details for ${job.name}`, exact: true });
    await expect(info).toHaveAttribute("aria-expanded", "true");
    await info.click();
    await expect(info).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("heading", { name: "Identity and source" })).toHaveCount(0);
    await info.click();
    await expect(info).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("heading", { name: "Identity and source" })).toBeVisible();
    await expect(page.locator(`.registry-detail-row td[colspan="${view === "policy" ? 6 : 5}"]`)).toHaveCount(1);
    if (view === "policy") expect(requests).toEqual([]);
    await info.click();
    await expect(info).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("heading", { name: "Identity and source" })).toHaveCount(0);
  }
});
