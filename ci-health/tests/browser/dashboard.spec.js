import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }, testInfo) => {
  const errors = [];
  page.browserErrors = errors;
  page.on("pageerror", error => errors.push(error.message));
  page.on("console", message => {
    if (message.type() === "error") errors.push(message.text());
  });
  if (testInfo.title.includes("while an uncached window loads")) {
    let releaseWindowResponse;
    const windowResponseGate = new Promise(resolve => { releaseWindowResponse = resolve; });
    page.releaseWindowResponse = releaseWindowResponse;
    await page.route("**/_dashboard/health/windows/1w", async route => {
      await windowResponseGate;
      await route.continue();
    });
  }
  await page.goto("/");
  await expect(page).toHaveURL(/\/presubmits(?:\?|$)/);
  await expect(page.getByRole("heading", { name: "HyperShift CI Health" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "Job", exact: true })).toBeVisible();
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute("href", "/assets/web/favicon.svg");
  await expect(page.getByRole("link", { name: "Job Registry API docs" })).toHaveAttribute("target", "_blank");
});

test.afterEach(async ({ page }) => {
  expect(page.browserErrors, "browser errors").toEqual([]);
});

test("renders the health dashboard and changes perspectives", async ({ page }) => {
  await expect(page.locator("tbody tr")).not.toHaveCount(0);

  const rate = page.locator(".rate-summary").first();
  await rate.hover();
  await expect(rate.getByRole("tooltip")).toBeVisible();
  await expect(rate.getByRole("tooltip")).toContainText("runs");
  await expect(page.locator(".rate-chart .rate-area").first()).toHaveAttribute("fill-opacity", "0.08");

  const graphPoint = page.locator(".sparkline .graph-hit").first();
  await graphPoint.hover();
  await expect(page.locator(".graph-tip")).toBeVisible();

  const jobBadge = page.locator(".job-type.has-tip").first();
  await jobBadge.hover();
  await expect(jobBadge.getByRole("tooltip")).toBeVisible();

  await expect(page.getByRole("link", { name: /Sippy analysis for/ }).first().locator("svg")).toBeVisible();
  await expect(page.getByRole("link", { name: /Prow history for/ }).first().locator("svg")).toBeVisible();
  const passRateBefore = await page.getByRole("columnheader", { name: "Pass Rate" }).boundingBox();
  const timelineBefore = await page.getByRole("columnheader", { name: "Last 2w" }).boundingBox();
  const registryInfo = page.getByRole("button", { name: /Registry details for/ }).first();
  const healthURL = page.url();
  await registryInfo.click();
  await expect(page.getByRole("heading", { name: "Identity and source" })).toBeVisible();
  expect(page.url()).toBe(healthURL);
  await expect(registryInfo).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByRole("link", { name: "View in Job Policy" })).toHaveCount(0);
  const passRateAfter = await page.getByRole("columnheader", { name: "Pass Rate" }).boundingBox();
  const timelineAfter = await page.getByRole("columnheader", { name: "Last 2w" }).boundingBox();
  expect(Math.abs(passRateAfter.x - passRateBefore.x)).toBeLessThan(0.1);
  expect(Math.abs(timelineAfter.x - timelineBefore.x)).toBeLessThan(0.1);

  const payloadTab = page.getByRole("button", { name: "Release Payload" });
  const payloadWidth = (await payloadTab.boundingBox()).width;
  await payloadTab.click();
  await expect(page.getByRole("columnheader", { name: "Release payload job" })).toBeVisible();
  await expect(page).toHaveURL(/\/payload(?:\?|$)/);
  expect(Math.abs((await payloadTab.boundingBox()).width - payloadWidth)).toBeLessThan(0.1);

  const componentTab = page.getByRole("button", { name: "Component Readiness" });
  const componentWidth = (await componentTab.boundingBox()).width;
  await componentTab.click();
  await expect(page.getByRole("columnheader", { name: "Component Readiness job" })).toBeVisible();
  await expect(page).toHaveURL(/\/component-readiness(?:\?|$)/);
  expect(Math.abs((await componentTab.boundingBox()).width - componentWidth)).toBeLessThan(0.1);

  await page.goBack();
  await expect(page).toHaveURL(/\/payload(?:\?|$)/);
  await expect(page.getByRole("columnheader", { name: "Release payload job" })).toBeVisible();
});

test("selects short windows and restores them from the URL", async ({ page }) => {
  for (const window of ["24h", "48h"]) {
    const button = page.getByRole("button", { name: window, exact: true });
    await button.click();
    await expect(button).toHaveClass(/active/);
    await expect(page.getByRole("columnheader", { name: `Last ${window}` })).toBeVisible();
    await expect.poll(() => new URL(page.url()).searchParams.get("window")).toBe(window);
    await page.reload();
    await expect(button).toHaveClass(/active/);
    await expect(page.getByRole("columnheader", { name: `Last ${window}` })).toBeVisible();
  }
});

test("shows HyperShift payload participation with its declared role and tested release", async ({ page }) => {
  await page.goto("/payload?release-newest=4.22&release-oldest=4.22");
  const older = page.locator(".periodic-row").filter({ has: page.getByRole("button", { name: "Registry details for periodic-ci-openshift-hypershift-release-4.22-periodics-e2e-aks-multi-x-ax", exact: true }) });
  await expect(older).toHaveCount(1);
  await expect(older.locator(".perspective-context")).toContainText("hypershift · informing");
  await expect(older.locator(".perspective-context")).toContainText("4.22.0-0.nightly-multi · blocking");
  await expect(older.locator('a[href$="/releasestream/hypershift"]')).toHaveText("payload status");
  await expect(older).not.toContainText("unknown architecture");

  await page.goto("/payload?release-newest=5.1&release-oldest=5.1");
  const future = page.locator(".periodic-row").filter({ has: page.getByRole("button", { name: "Registry details for periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aks", exact: true }) });
  await expect(future).toHaveCount(1);
  await expect(future.locator(".perspective-context")).toContainText("hypershift · blocking");
  await expect(future.locator(".perspective-context")).toContainText("5.1.0-0.ci · blocking");
  await expect(older).toHaveCount(0);
});

test("filters payload streams and restores multiple selections from the URL", async ({ page }) => {
  await page.goto("/payload?release-newest=4.22&release-oldest=4.22");
  const filter = page.getByLabel("Release stream filter");
  const menu = page.locator(".stream-menu");
  const rows = page.locator("tbody .periodic-row");
  await expect(rows).not.toHaveCount(0);
  const allCount = await rows.count();
  await expect(filter).toHaveText("Streams All");
  await filter.click();
  await expect(menu.getByRole("checkbox", { name: "All streams", exact: true })).toBeChecked();
  await menu.getByRole("checkbox", { name: "hypershift", exact: true }).check();
  await expect(rows).toHaveCount(4);
  await expect(filter).toHaveText("Streams hypershift");
  await expect(page.locator(".perspective-context")).not.toContainText(["4.22.0-0.nightly-multi"]);
  await expect.poll(() => new URL(page.url()).searchParams.getAll("stream")).toEqual(["hypershift"]);

  await menu.getByRole("checkbox", { name: "4.22.0-0.nightly-multi", exact: true }).check();
  await expect(filter).toHaveText("Streams 2 selected");
  const selectedCount = await rows.count();
  expect(selectedCount).toBeGreaterThanOrEqual(4);
  await page.reload();
  await expect(rows).toHaveCount(selectedCount);
  await expect(filter).toHaveText("Streams 2 selected");
  await filter.click();
  await expect(menu.getByRole("checkbox", { name: "hypershift", exact: true })).toBeChecked();
  await expect(menu.getByRole("checkbox", { name: "4.22.0-0.nightly-multi", exact: true })).toBeChecked();
  await menu.getByRole("checkbox", { name: "All streams", exact: true }).check();
  await expect(rows).toHaveCount(allCount);
  await expect(filter).toHaveText("Streams All");
  await expect.poll(() => new URL(page.url()).searchParams.has("stream")).toBe(false);
  await page.goBack();
  await expect(filter).toHaveText("Streams 2 selected");
  await page.getByRole("button", { name: "Presubmit", exact: true }).click();
  await expect(filter).toHaveCount(0);
});

test("keeps the current table visible while an uncached window loads", async ({ page }) => {
  await expect(page.getByRole("columnheader", { name: "Last 2w" })).toBeVisible();
  const requestedWindow = page.getByRole("button", { name: "1w", exact: true });
  await requestedWindow.click();

  await expect(requestedWindow).toHaveClass(/active/);
  await expect(page.getByRole("columnheader", { name: "Last 2w" })).toBeVisible();
  await expect(page.locator(".refresh-btn")).toHaveCount(0);

  page.releaseWindowResponse();
  await expect(page.getByRole("columnheader", { name: "Last 1w" })).toBeVisible();
});

test("filters and groups without a page navigation", async ({ page }) => {
  await expect(page.getByLabel("Grouping")).toContainText("Release → Platform");
  await expect(page.getByRole("button", { name: "2w", exact: true })).toHaveClass(/active/);
  await page.getByLabel("Platform filter").click();
  const platformOptions = page.locator('.platform-menu input[type="checkbox"]');
  const platforms = await platformOptions.evaluateAll(options => options.map(option => option.value));
  expect(platforms.length).toBeGreaterThan(1);
  const releases = await page.locator(".release-range-ticks span").allTextContents();
  expect(releases.length).toBeGreaterThan(1);
  for (let index = 1; index < releases.length; index++) {
    const previous = releases[index - 1].split(".").map(Number);
    const current = releases[index].split(".").map(Number);
    expect(previous[0] > current[0] || previous[0] === current[0] && previous[1] > current[1]).toBe(true);
  }

  await page.getByLabel("Grouping").click();
  await page.getByRole("radio", { name: "Release", exact: true }).check();
  const releaseGroups = page.locator("tr.category-row");
  await expect(releaseGroups.first()).toContainText(`Future (${releases[0]})`);
  await expect(releaseGroups.nth(1)).toContainText(`N-1 (${releases[1]})`);
  await expect(page).toHaveURL(/group=release/);

  await page.getByLabel("Grouping").click();
  await page.getByRole("radio", { name: "Release → Platform", exact: true }).check();
  const releaseBlocks = page.locator("tr.category-row.release-group-row");
  await expect(releaseBlocks.first()).toContainText(`Future (${releases[0]})`);
  await expect(page.locator("tr.release-block-spacer").first()).toBeVisible();
  await expect(page.locator("tr.release-block-spacer td").first()).toHaveCSS("height", "8px");
  await expect(page.locator("tr.subcategory-row.platform-group-row").first()).not.toBeEmpty();
  await expect.poll(() => new URL(page.url()).searchParams.has("group")).toBe(false);

  await page.getByLabel("Grouping").click();
  await page.getByRole("radio", { name: "Release", exact: true }).check();

  await page.getByLabel("Platform filter").click();
  await platformOptions.nth(0).check();
  await platformOptions.nth(1).check();
  await expect.poll(() => new URL(page.url()).searchParams.getAll("platform")).toEqual(platforms.slice(0, 2));
  await expect.poll(() => new URL(page.url()).searchParams.get("release-newest")).toBe(releases[0]);
  await expect.poll(() => new URL(page.url()).searchParams.get("release-oldest")).toBe(releases[Math.min(2, releases.length - 1)]);

  await page.reload();
  await expect(page.getByLabel("Grouping")).toContainText("Release");
  await expect(page.getByLabel("Platform filter")).toContainText("2 selected");
  await page.getByLabel("Platform filter").click();
  await expect(page.locator('.platform-menu input[type="checkbox"]:checked')).toHaveCount(2);
  await expect(page.locator(".release-range-ticks span").first()).toHaveText(releases[0]);
});

test("browses, filters, and deep-links the job registry", async ({ page }) => {
  await page.getByRole("button", { name: "Job Registry" }).click();
  const search = page.getByRole("searchbox", { name: "Search job registry" });
  await expect(search).toBeVisible();
  await expect(page.getByText(/^Updated /)).toBeVisible();
  await expect(page.locator(".refresh-btn")).toHaveCount(0);
  await expect(page.locator("tr.registry-job")).toHaveCount(100);

  await page.getByText("Type", { exact: true }).click();
  await page.getByRole("checkbox", { name: /Presubmit/ }).check();
  await expect.poll(() => new URL(page.url()).searchParams.getAll("type")).toEqual(["presubmit"]);
  await expect(page.getByRole("button", { name: /Type: Presubmit/ })).toBeVisible();

  await search.fill("pull-ci-openshift-hypershift-main-e2e-aks");
  const job = page.getByRole("link", { name: "pull-ci-openshift-hypershift-main-e2e-aks", exact: true });
  await expect(job).toBeVisible();
  await expect(page.locator(".registry-job-link").first()).toHaveText("pull-ci-openshift-hypershift-main-e2e-aks");
  await expect(page).toHaveURL(/\/registry(?:\?|$)/);
  await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe("pull-ci-openshift-hypershift-main-e2e-aks");
  await job.click();
  await expect(page.getByRole("heading", { name: "Presubmit behavior" })).toBeVisible();
  await expect(page.getByText("Sippy presubmit ingestion is enabled for the development branch.")).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get("job")).toBe("pull-ci-openshift-hypershift-main-e2e-aks");

  await page.reload();
  await expect(page.getByRole("searchbox", { name: "Search job registry" })).toHaveValue("pull-ci-openshift-hypershift-main-e2e-aks");
  await expect(page.getByRole("heading", { name: "Presubmit behavior" })).toBeVisible();

  await page.getByRole("link", { name: "Presubmit Health", exact: true }).first().click();
  await expect(page).toHaveURL(/\/presubmits\?.*job=pull-ci-openshift-hypershift-main-e2e-aks/);
  await expect(page.getByText("Showing CI Health context for")).toBeVisible();
  await expect(page.locator("tbody tr").filter({ hasText: "e2e-aks" }).first()).toBeVisible();
  await page.getByRole("link", { name: "Registry details", exact: true }).first().click();
  await expect(page.getByRole("heading", { name: "Presubmit behavior" })).toBeVisible();
  const selectedRow = page.locator("tr.registry-job.expanded");
  await expect(selectedRow).toBeVisible();
  await expect.poll(async () => {
    const rowBox = await selectedRow.boundingBox();
    return Math.abs(rowBox.y + rowBox.height / 2 - page.viewportSize().height / 2);
  }).toBeLessThan(2);
  await expect(selectedRow.locator(".registry-job-link")).toBeFocused();
  expect(await page.evaluate(() => localStorage.length)).toBe(0);
});

test("selects required optional and all presubmits and restores scope", async ({ page }) => {
  await page.goto("/presubmits?window=1w&release-newest=5.1&release-oldest=5.1");
  const scope = page.getByRole("group", { name: "Presubmit scope" });
  await expect(scope.getByRole("button", { name: "Required", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator('.presubmit-row[data-required="true"]')).not.toHaveCount(0);
  await expect(page.locator('.presubmit-row[data-required="false"]')).toHaveCount(0);

  await scope.getByRole("button", { name: "Optional", exact: true }).click();
  await expect(page.locator('.presubmit-row[data-required="true"]')).toHaveCount(0);
  await expect(page.locator('.presubmit-row[data-required="false"]')).not.toHaveCount(0);
  await expect(page.locator(".periodic-row")).not.toHaveCount(0);
  const autonode = page.locator('.presubmit-row[data-job-id="pull-ci-openshift-hypershift-main-e2e-aws-autonode"]');
  await expect(autonode).toContainText("Optional presubmit");
  await expect(autonode.locator(".rate-chart-cell")).toHaveText("No recorded runs");
  await expect(page.locator('.periodic-row[data-job-id="periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-autonode"]')).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get("presubmit-scope")).toBe("optional");
  await page.reload();
  await expect(scope.getByRole("button", { name: "Optional", exact: true })).toHaveAttribute("aria-pressed", "true");

  await scope.getByRole("button", { name: "All", exact: true }).click();
  await expect(page.locator('.presubmit-row[data-required="true"]')).not.toHaveCount(0);
  await expect(page.locator('.presubmit-row[data-required="false"]')).not.toHaveCount(0);
  await page.goBack();
  await expect(scope.getByRole("button", { name: "Optional", exact: true })).toHaveAttribute("aria-pressed", "true");
  await scope.getByRole("button", { name: "Required", exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.has("presubmit-scope")).toBe(false);
  await expect(page.locator('.presubmit-row[data-required="false"]')).toHaveCount(0);
});

test("explains optional Future scope and offers matching optional coverage", async ({ page }) => {
  await page.goto("/presubmits?presubmit-scope=optional&release-newest=4.22&release-oldest=4.22");
  await expect(page.locator(".presubmit-row")).toHaveCount(0);
  await expect(page.locator(".presubmit-empty")).toContainText("Optional presubmits are shown for Future (5.1) only.");
  await page.getByRole("button", { name: "Include Future (5.1)", exact: true }).click();
  await expect(page.locator('.presubmit-row[data-required="false"]')).not.toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get("release-newest")).toBe("5.1");
  await expect.poll(() => new URL(page.url()).searchParams.get("release-oldest")).toBe("4.22");
  await expect.poll(() => new URL(page.url()).searchParams.get("presubmit-scope")).toBe("optional");

  await page.goto("/presubmits?release-newest=5.1&release-oldest=5.1&platform=openstack");
  await expect(page.locator(".presubmit-row")).toHaveCount(0);
  await page.getByRole("button", { name: "Show optional", exact: true }).click();
  await expect(page.locator('.presubmit-row[data-required="false"]')).not.toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.getAll("platform")).toEqual(["openstack"]);
});

test("deep-links optional presubmits and displays mapping nuances", async ({ page }) => {
  const job = "pull-ci-openshift-hypershift-main-e2e-aws-ovn-conformance-techpreview";
  await page.goto(`/presubmits?window=1w&job=${job}`);
  await expect(page.getByRole("group", { name: "Presubmit scope" }).getByRole("button", { name: "Optional", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".presubmit-row")).toHaveCount(1);
  await expect(page.locator(".presubmit-row")).toContainText("Optional presubmit");
  const periodic = page.locator('.periodic-row[data-job-id="periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance-techpreview"]');
  await periodic.locator(".job-type.has-tip").hover();
  await expect(periodic.getByRole("tooltip")).toContainText("NLB tests");
  await page.screenshot({ path: "/tmp/ci-health-optional-presubmit.png" });
  await page.getByRole("group", { name: "Presubmit scope" }).getByRole("button", { name: "Required", exact: true }).click();
  await expect.poll(() => new URL(page.url()).searchParams.has("job")).toBe(false);
  await expect(page.locator('.presubmit-row[data-required="false"]')).toHaveCount(0);
});

test("scopes failure alerts and labels jobs without recorded runs", async ({ page }) => {
  await page.route("**/_dashboard/health/windows/*", async route => {
    const response = await route.fetch();
    const snapshot = await response.json();
    const required = snapshot.data.jobs.find(job => job.id === "pull-ci-openshift-hypershift-main-e2e-aws");
    const optional = snapshot.data.jobs.find(job => job.id === "pull-ci-openshift-hypershift-main-e2e-aws-metrics");
    const noRuns = snapshot.data.jobs.find(job => job.id === "pull-ci-openshift-hypershift-main-e2e-aws-autonode");
    optional.runs = 0;
    noRuns.runs = 0;
    snapshot.data.alerts = [{ test_name: "Shared test failure", failure_count: 3, jobs: [
      { id: required.id, name: required.name, target_release: required.target_release, required: true, failure_count: 1 },
      { id: optional.id, name: optional.name, target_release: optional.target_release, required: false, failure_count: 2 },
    ] }];
    await route.fulfill({ response, json: snapshot });
  });
  await page.goto("/presubmits?window=1w&release-newest=5.1&release-oldest=5.1");
  const alerts = page.locator(".alert-banner");
  await expect(alerts).toContainText("Required presubmits:");
  await expect(alerts).toContainText("1 failure");
  await expect(alerts).not.toContainText("e2e-aws-metrics");
  const scope = page.getByRole("group", { name: "Presubmit scope" });
  await scope.getByRole("button", { name: "Optional", exact: true }).click();
  await expect(alerts).toContainText("Optional presubmits:");
  await expect(alerts).toContainText("2 failures");
  await expect(alerts).toContainText("[Optional");
  await expect(alerts).not.toContainText("[Required");
  const metrics = page.locator('.presubmit-row[data-job-id="pull-ci-openshift-hypershift-main-e2e-aws-metrics"]');
  await expect(metrics.locator(".rate-chart-cell")).toHaveText("No recorded runs");
  await expect(page.locator('.presubmit-row[data-job-id="pull-ci-openshift-hypershift-main-e2e-aws-autonode"] .rate-chart-cell')).toHaveText("No recorded runs");
  await scope.getByRole("button", { name: "All", exact: true }).click();
  await expect(alerts).toContainText("3 failures");
  await expect(alerts).toContainText("[Required");
  await expect(alerts).toContainText("[Optional");
  await page.getByLabel("Platform filter").click();
  await page.getByRole("checkbox", { name: "openstack", exact: true }).check();
  await expect(alerts).toHaveCount(0);
});

test("rejects partial health responses", async ({ page }) => {
  await page.route("**/_dashboard/health/windows/*", async route => {
    const response = await route.fetch();
    const snapshot = await response.json();
    snapshot.complete = false;
    await route.fulfill({ response, json: snapshot });
  });
  await page.goto("/presubmits");
  await expect(page.locator(".loading.error")).toHaveText("Incomplete health report rejected; a complete dataset is required.");
  await expect(page.locator(".presubmit-row")).toHaveCount(0);
});
