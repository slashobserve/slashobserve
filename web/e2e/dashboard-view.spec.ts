import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  panelDataResponse,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E overview")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function localMinute(ms: number): string {
  const date = new Date(ms)
  const pad = (value: number) => String(value).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function localSecond(ms: number): string {
  const date = new Date(ms)
  return `${localMinute(ms)}:${String(date.getSeconds()).padStart(2, "0")}`
}

test("renders every panel with data", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expectPanelsReady(page)

  const logs = panel(page, "Log rows by service")
  await expect(
    logs.getByRole("columnheader", { name: "service" })
  ).toBeVisible()
  expect(await logs.getByRole("row").count()).toBeGreaterThan(1)
  await expect(page).toHaveTitle(`${spec.title} - SlashObserve`)
})

test("changing the time range updates the URL and reloads panels", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  await page.getByRole("button", { name: /^Time range/ }).click()
  const reloaded = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.get("from") === "now-1h"
  )
  await page.getByRole("button", { name: "Last 1 hour", exact: true }).click()
  await reloaded
  await expect(page).toHaveURL(/from=now-1h&to=now/)

  await page.reload()
  await expect(
    page.getByRole("button", { name: "Time range: Last 1 hour" })
  ).toBeVisible()
  await expectPanelsReady(page)
})

test("labels a non-preset relative range", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id, "?from=now-20m&to=now")
  await expect(
    page.getByRole("button", { name: "Time range: Last 20 minutes" })
  ).toBeVisible()
  await expectPanelsReady(page)
})

test("applies a custom absolute range", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const trigger = page.getByRole("button", { name: /^Time range/ })
  await trigger.click()
  const from = page.getByRole("textbox", { name: "From" })
  const to = page.getByRole("textbox", { name: "To" })
  await from.fill("yesterday")
  await page.getByRole("button", { name: "Apply" }).click()
  await expect(page.getByRole("alert")).toBeVisible()
  expect(page.url()).not.toContain("from=")

  const now = Date.now()
  await from.fill(localMinute(now - 30 * 60_000))
  await to.fill(localSecond(now))
  const isAbsolute = (url: URL) =>
    /^\d{13}$/.test(url.searchParams.get("to") ?? "")
  const rps = panelDataResponse(page, "rps", isAbsolute)
  const errorRate = panelDataResponse(page, "error_rate", isAbsolute)
  await page.getByRole("button", { name: "Apply" }).click()
  await expect(page).toHaveURL(/from=\d{13}&to=\d{13}/)
  await Promise.all([rps, errorRate])
  await expectPanelsReady(page)
})

test("a variable filters the series and survives reload", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const pod = page.getByRole("combobox", { name: "Pod" })
  await pod.click()
  const filtered = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.getAll("var-pod").join(",") === "checkout-0"
  )
  await page.getByRole("option", { name: "checkout-0" }).click()
  await expect(page).toHaveURL(/var-pod=checkout-0(&|$)/)
  const body = (await (await filtered).json()) as {
    frames: { labels?: { pod?: string } }[]
  }
  expect(body.frames.length).toBeGreaterThan(0)
  for (const frame of body.frames) expect(frame.labels?.pod).toBe("checkout-0")
  await page.keyboard.press("Escape")

  await page.reload()
  const chips = page.locator('[data-slot="combobox-chips"]')
  await expect(chips).toContainText("checkout-0")
  await expectPanelsReady(page)

  await page.getByRole("combobox", { name: "Pod" }).click()
  await page.getByRole("option", { name: "All", exact: true }).click()
  await expect(page).toHaveURL(/var-pod=%24__all/)
})

function dependentVariablesSpec() {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E dependent variables")
  const datasource = "prom"
  spec.variables = [
    {
      name: "service",
      label: "Service",
      type: "query",
      datasource,
      query: "label_values(http_requests_total, service)",
    },
    {
      name: "route",
      label: "Route",
      type: "query",
      datasource,
      query: 'label_values(http_requests_total{service="$service"}, route)',
    },
    {
      name: "pod",
      label: "Pod",
      type: "query",
      multi: true,
      datasource,
      query: 'label_values(http_requests_total{service="$service"}, pod)',
    },
  ]
  spec.panels = [
    {
      ...spec.panels[0],
      queries: [
        {
          query:
            'sum by (pod) (rate(http_requests_total{service="$service",route="$route",pod=~"$pod"}[$__rate_interval]))',
        },
      ],
    },
  ]
  return spec
}

async function holdVariableValues(page: Page) {
  let release: () => void = () => undefined
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route("**/variables/*/values*", async (route) => {
    await gate
    await route.continue().catch(() => undefined)
  })
  return async () => {
    release()
    await page.unroute("**/variables/*/values*")
  }
}

async function labelPositions(page: Page) {
  const positions: number[] = []
  for (const name of ["Service", "Route", "Pod"]) {
    const box = await page
      .locator("label", { hasText: new RegExp(`^${name}$`) })
      .boundingBox()
    positions.push(Math.round(box?.x ?? -1))
  }
  return positions
}

test("dependent variables stay in place while they reload", async ({
  page,
  request,
}) => {
  const { id } = await createDashboard(request, dependentVariablesSpec())
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const route = page.getByRole("combobox", { name: "Route" })
  const pod = page.getByRole("combobox", { name: "Pod" })
  const idle = await labelPositions(page)
  expect(new Set(idle).size).toBe(3)

  let release = await holdVariableValues(page)
  const reloaded = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.get("from") === "now-1h"
  )
  await page.getByRole("button", { name: /^Time range/ }).click()
  await page.getByRole("button", { name: "Last 1 hour", exact: true }).click()
  await expect(route).toBeDisabled()
  await expect(pod).toBeDisabled()
  await expect(route).toHaveAttribute("aria-busy", "true")
  expect(await labelPositions(page)).toEqual(idle)
  await reloaded
  await release()
  await expect(route).toBeEnabled()
  await expect(pod).toBeEnabled()
  expect(await labelPositions(page)).toEqual(idle)

  release = await holdVariableValues(page)
  await page.getByRole("combobox", { name: "Service" }).click()
  const options = page.getByRole("option")
  await expect(options.first()).toBeVisible()
  const count = await options.count()
  expect(count).toBeGreaterThan(1)
  for (let index = 0; index < count; index++) {
    const option = options.nth(index)
    if ((await option.getAttribute("aria-selected")) !== "true") {
      await option.click()
      break
    }
  }
  await expect(route).toBeDisabled()
  await expect(pod).toBeDisabled()
  await expect(route).toContainText("Loading...")
  await expect(page.getByPlaceholder("Loading...")).toBeVisible()
  await release()
  await expect(route).toBeEnabled()
  await expect(pod).toBeEnabled()
  await expectPanelsReady(page)
})

test("auto-refresh fetches new points without reload", async ({
  page,
  request,
}) => {
  test.setTimeout(60_000)
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const firstResponse = panelDataResponse(page, "rps")
  await page.getByRole("combobox", { name: "Auto-refresh" }).click()
  await page.getByRole("option", { name: "5s", exact: true }).click()
  await expect(page).toHaveURL(/refresh=5s/)
  const first = await firstResponse
  const second = await page.waitForResponse(
    (response) =>
      response.url().includes("/panels/rps/data") &&
      response.ok() &&
      response !== first,
    { timeout: 15_000 }
  )

  type Data = { frames: { fields: { type: string; values: number[] }[] }[] }
  const latestTime = (data: Data) =>
    Math.max(
      ...data.frames.flatMap((frame) =>
        frame.fields
          .filter((field) => field.type === "time")
          .flatMap((field) => field.values)
      )
    )
  expect(latestTime((await second.json()) as Data)).toBeGreaterThan(
    latestTime((await first.json()) as Data)
  )
})

test("shows a panel error and recovers", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await page.route("**/panels/rps/data*", (route) =>
    route.fulfill({
      status: 504,
      contentType: "application/json",
      body: JSON.stringify({ error: "query timed out", code: "query_timeout" }),
    })
  )
  await openDashboard(page, id)
  const rps = panel(page, "Checkout requests per second")
  await expect(rps).toHaveAttribute("data-state", "error", { timeout: 10_000 })
  await expect(rps.getByText("Query timed out", { exact: true })).toBeVisible()

  await page.unroute("**/panels/rps/data*")
  await rps.getByRole("button", { name: "Retry" }).click()
  await expect(rps).toHaveAttribute("data-state", "ready", { timeout: 30_000 })
  consoleErrors.length = 0
})

test("shows not found for an unknown dashboard", async ({
  page,
  consoleErrors,
}) => {
  await page.goto("/dashboards/00000000-0000-0000-0000-000000000000")
  await expect(
    page.getByRole("heading", { name: "Dashboard not found" })
  ).toBeVisible()
  const back = page.getByRole("link", { name: "Back to dashboards" })
  await expect(back).toHaveAttribute("href", "/")
  await back.click()
  await expect(page).toHaveURL("/")
  consoleErrors.length = 0
})

test("shows a load problem for a version that fails and retries it", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await page.route("**/api/v1/dashboards/*/versions/1", (route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: "database unavailable", code: "internal" }),
    })
  )
  await page.goto(`/dashboards/${id}?version=1`)
  await expect(page.getByText("Could not load version 1")).toBeVisible()
  await expect(page.getByText("Database unavailable.")).toBeVisible()
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" })
  ).toBeVisible()
  await expect(page.getByText("Something went wrong")).toHaveCount(0)

  await page.unroute("**/api/v1/dashboards/*/versions/1")
  await page.getByRole("button", { name: "Retry" }).click()
  await expect(versionButton(page)).toHaveText("v1 of 2")
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  consoleErrors.length = 0
})

test("views an older version and returns to latest", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  const renamed = { ...spec, title: `${spec.title} renamed` }
  await addVersion(request, id, renamed, 1)

  await openDashboard(page, id)
  await versionButton(page).click()
  await page.getByRole("menuitemradio", { name: /^v1\b/ }).click()
  await expect(page).toHaveURL(/version=1/)
  await expect(versionButton(page)).toHaveText("v1 of 2")
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)

  await versionButton(page).click()
  await page.getByRole("menuitemradio", { name: /Latest/ }).click()
  await expect(page).not.toHaveURL(/version=/)
  await expect(versionButton(page)).toHaveText(/^(Live|Connecting) · v2$/)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    renamed.title
  )
})

test("is usable from the keyboard", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const trigger = page.getByRole("button", { name: /^Time range/ })
  await trigger.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("textbox", { name: "From" })).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("textbox", { name: "From" })).toHaveCount(0)
  await expect(trigger).toBeFocused()

  const visited: string[] = []
  for (let step = 0; step < 80 && !visited.includes("logs"); step++) {
    await page.keyboard.press("Tab")
    const menu = await page.evaluate(
      () => document.activeElement?.getAttribute("data-panel-menu") ?? null
    )
    if (menu) visited.push(menu)
  }
  expect(visited).toContain("rps")
  expect(visited).toContain("logs")
  expect(visited.indexOf("rps")).toBeLessThan(visited.indexOf("logs"))
})

test("never exposes datasource connection details", async ({
  page,
  request,
}) => {
  const bodies: string[] = []
  const reads: Promise<void>[] = []
  page.on("response", (response) => {
    const type = response.headers()["content-type"] ?? ""
    if (response.url().includes("/api/") && type.includes("json")) {
      reads.push(
        response.text().then(
          (text) => void bodies.push(text),
          () => undefined
        )
      )
    }
  })
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await Promise.all(reads)

  expect(bodies.length).toBeGreaterThan(0)
  for (const body of bodies) {
    for (const secret of [
      "prometheus:9090",
      "clickhouse:9000",
      "clickhouse:8123",
      "slashobserve_ro",
    ]) {
      expect(body).not.toContain(secret)
    }
  }
})

test("falls back to a single column on narrow screens", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await page.setViewportSize({ width: 600, height: 900 })
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const articles = page.locator("article[data-panel-id]")
  const count = await articles.count()
  expect(count).toBe(4)
  const xs = new Set<number>()
  for (let index = 0; index < count; index++) {
    const box = await articles.nth(index).boundingBox()
    expect(box).not.toBeNull()
    xs.add(Math.round(box?.x ?? -1))
  }
  expect(xs.size).toBe(1)
})

test("renders row children and marks panels it cannot show yet", async ({
  page,
  request,
}) => {
  const spec = loadSpec("evolved")
  spec.title = uniqueTitle("E2E evolved")
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)

  const article = (panelId: string) =>
    page.locator(`article[data-panel-id="${panelId}"]`)
  await article("p_up").scrollIntoViewIfNeeded()
  await expect(article("p_up")).toHaveAttribute("data-state", "ready", {
    timeout: 30_000,
  })
  for (const panelId of ["p_note", "p_world"]) {
    await expect(article(panelId)).toHaveAttribute("data-state", "unsupported")
    await expect(article(panelId)).toContainText(
      "This panel type is not shown yet."
    )
  }
  await expect(article("r_prom")).toHaveCount(0)
  await expect(
    page.getByRole("heading", { name: "Prometheus", level: 2 })
  ).toBeVisible()
})
