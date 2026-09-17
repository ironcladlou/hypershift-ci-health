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

test("keeps the current table visible while an uncached window loads", async ({ page }) => {
  await expect(page.getByRole("columnheader", { name: "Last 2w" })).toBeVisible();
  const requestedWindow = page.getByRole("button", { name: "1w", exact: true });
  await requestedWindow.click();

  await expect(requestedWindow).toHaveClass(/active/);
  await expect(page.getByRole("columnheader", { name: "Last 2w" })).toBeVisible();
  await expect(page.locator(".refresh-btn")).toHaveClass(/spinning/);

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

test("loads and searches the job registry", async ({ page }) => {
  await page.getByRole("button", { name: "Job Registry" }).click();
  const search = page.getByRole("searchbox", { name: "Fuzzy search job registry" });
  await expect(search).toBeVisible();
  await search.fill("karpenter");
  await expect(page.locator("tr.registry-job").first()).toBeVisible();
  await expect(page).toHaveURL(/\/registry(?:\?|$)/);
  await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe("karpenter");
  expect([...new URL(page.url()).searchParams.keys()]).toEqual(["q"]);

  await page.reload();
  await expect(page.getByRole("searchbox", { name: "Fuzzy search job registry" })).toHaveValue("karpenter");
  await expect(page.locator("tr.registry-job").first()).toBeVisible();
  expect(await page.evaluate(() => localStorage.length)).toBe(0);
});
