import type { APIRequestContext, Locator, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  panelDataResponse,
} from "./dashboards"
import { expect, test } from "./fixtures"

const rpsTitle = "Checkout requests per second"

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E panels")
  const { id } = await createDashboard(request, spec)
  return id
}

function legendButton(article: Locator, name: string) {
  return article.getByRole("button", { name, exact: true })
}

function swatchColor(article: Locator, name: string) {
  return legendButton(article, name)
    .locator('[data-slot="legend-swatch"]')
    .evaluate((element) => getComputedStyle(element).color)
}

async function choosePod(page: Page, option: string, selected: string) {
  await page.getByRole("combobox", { name: "Pod" }).click()
  const response = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.getAll("var-pod").join(",") === selected
  )
  await page.getByRole("option", { name: option, exact: true }).click()
  await response
  await page.keyboard.press("Escape")
}

function seriesPixels(canvas: Locator, token: string) {
  return canvas.evaluate((element, name) => {
    const swatch = document.createElement("canvas").getContext("2d")!
    swatch.fillStyle = getComputedStyle(document.documentElement)
      .getPropertyValue(name)
      .trim()
    swatch.fillRect(0, 0, 1, 1)
    const [red, green, blue] = swatch.getImageData(0, 0, 1, 1).data
    const source = element as HTMLCanvasElement
    const { data } = source
      .getContext("2d")!
      .getImageData(0, 0, source.width, source.height)
    let count = 0
    for (let offset = 0; offset < data.length; offset += 4) {
      if (
        data[offset + 3] === 255 &&
        Math.abs(data[offset] - red) < 24 &&
        Math.abs(data[offset + 1] - green) < 24 &&
        Math.abs(data[offset + 2] - blue) < 24
      ) {
        count++
      }
    }
    return count
  }, token)
}

test("draws every panel type with data", async ({ page, request }) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  await expect(rps.locator("canvas")).toHaveCount(1)
  await expect(legendButton(rps, "checkout-0")).toBeVisible()
  await expect(legendButton(rps, "checkout-1")).toBeVisible()

  await expect(panel(page, "Checkout error rate").getByText(/%$/)).toBeVisible()

  const meters = panel(page, "Requests by pod").getByRole("meter")
  await expect(meters).toHaveCount(2)
  for (let index = 0; index < 2; index++) {
    await expect(meters.nth(index)).toHaveAttribute("aria-valuetext", /\S/)
  }

  const logs = panel(page, "Log rows by service")
  expect(await logs.getByRole("row").count()).toBeGreaterThan(1)
})

test("shows a crosshair tooltip and toggles series", async ({
  page,
  request,
}) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  await rps.locator(".u-over").hover()
  const tooltip = rps.locator('[data-slot="chart-tooltip"]')
  await expect(tooltip).toBeVisible()
  await expect(tooltip).toContainText(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}/)
  await expect(tooltip.locator(".ts-tooltip-row")).toHaveCount(2)

  const button = legendButton(rps, "checkout-1")
  await expect(button).toHaveAttribute("aria-pressed", "true")
  await button.click()
  await expect(button).toHaveAttribute("aria-pressed", "false")
  await button.click()
  await expect(button).toHaveAttribute("aria-pressed", "true")
})

test("keeps the tooltip inside a short panel with many series", async ({
  page,
  request,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E many series")
  spec.variables = []
  spec.panels = [
    {
      id: "many",
      type: "timeseries",
      title: "Requests by service, route and pod",
      gridPos: { x: 0, y: 0, w: 12, h: 4 },
      datasource: "prom",
      queries: [
        {
          query:
            "sum by (service, route, pod) (rate(http_requests_total[$__rate_interval]))",
          legend: "{{service}} {{route}} {{pod}}",
        },
      ],
      unit: "reqps",
    },
  ]
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const many = panel(page, "Requests by service, route and pod")
  await many.locator(".u-over").hover()
  const tooltip = many.locator('[data-slot="chart-tooltip"]')
  await expect(tooltip).toBeVisible()
  const more = tooltip.locator(".ts-tooltip-more")
  await expect(more).toHaveText(/^[1-9]\d* more$/)
  const hidden = Number((await more.textContent())!.split(" ")[0])
  expect(await tooltip.locator(".ts-tooltip-row").count()).toBe(9 - hidden)

  const article = await many.boundingBox()
  const box = await tooltip.boundingBox()
  expect(article).not.toBeNull()
  expect(box).not.toBeNull()
  expect(box!.y).toBeGreaterThanOrEqual(article!.y)
  expect(box!.y + box!.height).toBeLessThanOrEqual(article!.y + article!.height)
  expect(box!.x).toBeGreaterThanOrEqual(article!.x)
  expect(box!.x + box!.width).toBeLessThanOrEqual(article!.x + article!.width)
})

test("zooms by dragging", async ({ page, request }) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const over = panel(page, rpsTitle).locator(".u-over")
  const box = await over.boundingBox()
  expect(box).not.toBeNull()
  const { x, y, width, height } = box!
  await page.mouse.move(x + width / 3, y + height / 2)
  await page.mouse.down()
  await page.mouse.move(x + (2 * width) / 3, y + height / 2, { steps: 10 })
  await page.mouse.up()

  await expect(page).toHaveURL(/from=\d{13}&to=\d{13}/)
  const search = new URL(page.url()).searchParams
  const span = Number(search.get("to")) - Number(search.get("from"))
  expect(span).toBeGreaterThan(0)
  expect(span).toBeLessThan(15 * 60_000)
})

test("switches between chart and table view", async ({ page, request }) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  const menu = rps.getByRole("button", { name: `Panel menu: ${rpsTitle}` })
  await menu.click()
  await page.getByRole("menuitem", { name: "Show table view" }).click()
  await expect(rps.getByRole("columnheader", { name: "pod" })).toBeVisible()
  await expect(rps.locator("canvas")).toHaveCount(0)

  await menu.click()
  await page.getByRole("menuitem", { name: "Show chart" }).click()
  await expect(rps.locator("canvas")).toHaveCount(1)
})

test("renders in dark mode without leaking canvases", async ({
  page,
  request,
}) => {
  await page.emulateMedia({ colorScheme: "light" })
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  for (const choice of ["Dark", "Light", "Dark", "Light", "Dark"]) {
    await page.getByRole("button", { name: "Theme" }).click()
    await page.getByRole("menuitemradio", { name: choice }).click()
    if (choice === "Dark") {
      await expect(page.locator("html")).toHaveClass(/\bdark\b/)
    } else {
      await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)
    }
    await expect(rps.locator("canvas")).toHaveCount(1)
  }
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  await expectPanelsReady(page)

  await page.getByRole("button", { name: "Theme" }).click()
  await page.getByRole("menuitemradio", { name: "System" }).click()
})

test("keeps series colors when a variable filters series", async ({
  page,
  request,
}) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  const first = await swatchColor(rps, "checkout-1")
  expect(first).not.toBe(await swatchColor(rps, "checkout-0"))

  await choosePod(page, "checkout-1", "checkout-1")
  await expect(rps.getByRole("group", { name: "Series" })).toHaveCount(0)
  await expect(rps.locator("canvas")).toHaveCount(1)

  await choosePod(page, "All", "$__all")

  await expect(legendButton(rps, "checkout-0")).toBeVisible()
  expect(await swatchColor(rps, "checkout-1")).toBe(first)
  expect(await swatchColor(rps, "checkout-0")).not.toBe(first)
})

test("keeps a hidden series hidden across the table view", async ({
  page,
  request,
}) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  const button = legendButton(rps, "checkout-1")
  const color = await swatchColor(rps, "checkout-1")
  await button.click()
  await expect(button).toHaveAttribute("aria-pressed", "false")

  const menu = rps.getByRole("button", { name: `Panel menu: ${rpsTitle}` })
  await menu.click()
  await page.getByRole("menuitem", { name: "Show table view" }).click()
  await expect(rps.locator("canvas")).toHaveCount(0)
  await menu.click()
  await page.getByRole("menuitem", { name: "Show chart" }).click()

  await expect(rps.locator("canvas")).toHaveCount(1)
  await expect(legendButton(rps, "checkout-1")).toHaveAttribute(
    "aria-pressed",
    "false"
  )
  expect(await swatchColor(rps, "checkout-1")).toBe(color)
})

test("draws the only remaining series even if it was hidden", async ({
  page,
  request,
}) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  await legendButton(rps, "checkout-1").click()
  await choosePod(page, "checkout-1", "checkout-1")
  await expect(rps.getByRole("group", { name: "Series" })).toHaveCount(0)

  await expect
    .poll(() => seriesPixels(rps.locator("canvas"), "--series-2"))
    .toBeGreaterThan(50)
})

test("shows a series again after it left and came back", async ({
  page,
  request,
}) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  await legendButton(rps, "checkout-1").click()
  await expect(legendButton(rps, "checkout-1")).toHaveAttribute(
    "aria-pressed",
    "false"
  )
  await choosePod(page, "checkout-0", "checkout-0")
  await choosePod(page, "All", "$__all")

  await expect(legendButton(rps, "checkout-1")).toHaveAttribute(
    "aria-pressed",
    "true"
  )
})

test("labels each query's series with its own legend", async ({
  page,
  request,
}) => {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E two queries")
  spec.variables = []
  spec.panels = [
    {
      id: "two",
      type: "timeseries",
      title: "Two queries",
      gridPos: { x: 0, y: 0, w: 12, h: 8 },
      datasource: "prom",
      queries: [
        { query: "count(up)", legend: "all" },
        { query: 'count(up{job=~".+"})', legend: "job" },
      ],
    },
  ]
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const two = panel(page, "Two queries")
  await expect(legendButton(two, "all")).toBeVisible()
  await expect(legendButton(two, "job")).toBeVisible()
})
