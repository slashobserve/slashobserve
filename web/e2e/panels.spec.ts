import type { APIRequestContext, Locator } from "@playwright/test"

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

  await page.getByRole("combobox", { name: "Pod" }).click()
  const filtered = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.getAll("var-pod").join(",") === "checkout-1"
  )
  await page.getByRole("option", { name: "checkout-1" }).click()
  await filtered
  await page.keyboard.press("Escape")
  await expect(rps.getByRole("group", { name: "Series" })).toHaveCount(0)
  await expect(rps.locator("canvas")).toHaveCount(1)

  await page.getByRole("combobox", { name: "Pod" }).click()
  const all = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.getAll("var-pod").join(",") === "$__all"
  )
  await page.getByRole("option", { name: "All", exact: true }).click()
  await all
  await page.keyboard.press("Escape")

  await expect(legendButton(rps, "checkout-0")).toBeVisible()
  expect(await swatchColor(rps, "checkout-1")).toBe(first)
  expect(await swatchColor(rps, "checkout-0")).not.toBe(first)
})
