import type { APIRequestContext, Page } from "@playwright/test"

import { createDashboard, getDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectSavedVersion,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
import { draftPreviewRejectionLogged, expect, test } from "./fixtures"

const selectAll = process.platform === "darwin" ? "Meta+A" : "Control+A"
const rpsTitle = "Shop requests per second"
const savedQuery =
  'sum(rate(traces_span_metrics_calls_total{service_name="checkout",span_kind="SPAN_KIND_SERVER"}[$__rate_interval]))'
const saveVersionButton = /^Save as version \d+$/
const expectedLint429: string[] = []

test.beforeEach(({ page }) => {
  expectedLint429.length = 0
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      message.text().includes("status of 429") &&
      message.location().url.endsWith("/lint")
    ) {
      expectedLint429.push(message.text())
    }
  })
})

test.afterEach(({ consoleErrors }) => {
  for (const text of expectedLint429) {
    const index = consoleErrors.indexOf(text)
    if (index >= 0) consoleErrors.splice(index, 1)
  }
})

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E query")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function sheet(page: Page) {
  return page.getByRole("complementary", { name: /^Edit / })
}

async function openFromMenu(page: Page, title: string) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click()
}

function firstQuery(panel: Record<string, unknown> | undefined): unknown {
  return (panel?.queries as Array<{ query: string }> | undefined)?.[0]?.query
}

async function replaceQuery(page: Page, text: string) {
  const content = sheet(page).locator(".cm-content")
  await content.click()
  await page.keyboard.press(selectAll)
  await page.keyboard.insertText(text)
  await expect(content).toHaveText(text)
}

test("shows a lint error inline", async ({ page, request, consoleErrors }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, "sum(rate(")
  await expect(sheet(page).locator(".cm-lintRange-error").first()).toBeVisible({
    timeout: 5_000,
  })
  await page.keyboard.press("Escape")
  await expect(page.locator(".cm-tooltip-autocomplete")).toHaveCount(0)
  await expect(sheet(page).locator(".cm-lint-line").first()).toBeVisible()
  await expect(sheet(page).getByText(/^Error:/)).toHaveCount(1)
  await draftPreviewRejectionLogged(consoleErrors)
})

test("saves an edited query as a new version", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, savedQuery)
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  await expectSavedVersion(page, 2)
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=/)
  await expect(versionButton(page)).toHaveText(/v2$/)

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(2)
  const rps = saved.spec.panels.find((item) => item.id === "rps")
  expect(firstQuery(rps)).toBe(savedQuery)
  await expect(panel(page, rpsTitle)).toHaveAttribute("data-state", "ready", {
    timeout: 30_000,
  })
})

test("blocks saving a query the server cannot run", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, "sum(rate(")
  await draftPreviewRejectionLogged(consoleErrors)

  await expect(
    sheet(page).getByRole("button", { name: saveVersionButton })
  ).toHaveAttribute("aria-disabled", "true")
  await expect(sheet(page).getByText("Fix the query to save.")).toBeVisible()
  await expect(sheet(page).getByText("This panel was rejected:")).toHaveCount(0)
  expect((await getDashboard(request, id)).latestVersion).toBe(1)
})
