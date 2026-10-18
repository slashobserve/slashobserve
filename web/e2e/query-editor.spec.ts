import type { APIRequestContext, Page } from "@playwright/test"

import {
  addVersion,
  createDashboard,
  getDashboard,
  loadSpec,
  uniqueTitle,
} from "./api"
import { openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const selectAll = process.platform === "darwin" ? "Meta+A" : "Control+A"
const rpsTitle = "Checkout requests per second"
const savedQuery =
  'sum(rate(http_requests_total{service="checkout"}[$__rate_interval]))'

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E query")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function sheet(page: Page) {
  return page.getByRole("dialog")
}

async function openFromMenu(page: Page, title: string) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Edit query" }).click()
}

async function replaceQuery(page: Page, text: string) {
  const content = sheet(page).locator(".cm-content")
  await content.click()
  await page.keyboard.press(selectAll)
  await page.keyboard.insertText(text)
  await expect(content).toHaveText(text)
}

test("loads PromQL highlighting", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await expect(page).toHaveURL(/panel=rps&tab=query/)
  const editor = sheet(page).locator(".cm-editor")
  await expect(editor).toBeVisible()
  await expect
    .poll(() => editor.locator('.cm-content [class*="tok-"]').count())
    .toBeGreaterThan(0)
  await expect(
    sheet(page).getByRole("textbox", {
      name: "Query for Checkout requests per second",
    })
  ).toBeVisible()
})

test("loads SQL highlighting", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, "Log rows by service")

  await expect(page).toHaveURL(/panel=logs&tab=query/)
  await expect(sheet(page).locator(".cm-editor")).toBeVisible()
  await expect(
    sheet(page).locator(".cm-content .tok-keyword").first()
  ).toHaveText("SELECT")
})

test("shows a lint error inline", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, "sum(rate(")
  await expect(sheet(page).locator(".cm-lintRange-error").first()).toBeVisible({
    timeout: 5_000,
  })
  const item = sheet(page)
    .getByRole("button", { name: /^Error:/ })
    .first()
  await expect(item).toBeVisible()
  await item.click()
  await expect(sheet(page).locator(".cm-content")).toBeFocused()
})

test("saves an edited query as a new version", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, savedQuery)
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(page.getByText("Saved version 2")).toBeVisible()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=/)
  await expect(page.getByText("v2", { exact: true })).toBeVisible()

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(2)
  const rps = saved.spec.panels.find((item) => item.id === "rps")
  expect(rps?.query).toBe(savedQuery)
  await expect(panel(page, rpsTitle)).toHaveAttribute("data-state", "ready", {
    timeout: 30_000,
  })
})

test("rejects a query the server cannot run", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, 'up{job="does-not-exist"}')
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(sheet(page).getByText("This query was rejected:")).toBeVisible()
  await expect(sheet(page)).toBeVisible()
  expect((await getDashboard(request, id)).latestVersion).toBe(1)

  const rejected = consoleErrors.filter((message) =>
    message.includes("status of 422")
  )
  expect(rejected).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(rejected[0]), 1)
})

test("closes with Escape and returns focus", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await expect(sheet(page).locator(".cm-editor")).toBeVisible()
  await sheet(page).getByRole("textbox", { name: "Title" }).focus()
  await page.keyboard.press("Escape")

  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=|tab=/)
  await expect(page.locator('[data-panel-menu="rps"]')).toBeFocused()
})

test("opens directly from a link", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await page.goto(`/dashboards/${id}?panel=logs&tab=query`)

  await expect(sheet(page)).toBeVisible()
  await expect(
    sheet(page).getByRole("heading", { name: "Log rows by service" })
  ).toBeVisible()
  const logs = spec.panels.find((item) => item.id === "logs")
  await expect(sheet(page).locator(".cm-content")).toContainText(
    String(logs?.query).slice(0, 30)
  )
})

test("is read-only on an old version", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&tab=query`)

  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toBeVisible()
  const content = sheet(page).locator(".cm-content")
  const before = await content.textContent()
  await content.click({ force: true })
  await page.keyboard.type("zzz")
  await expect(content).toHaveText(before ?? "")
})

test("keeps an unsaved query when a newer version arrives", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  const renamed = `${spec.title} renamed`
  await addVersion(request, id, { ...spec, title: renamed }, 1)
  await expect(
    sheet(page).getByText("Version 2 was saved while you were editing.")
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()
  await expect(page.getByText("Saved version 3")).toBeVisible()
  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(3)
  expect(saved.title).toBe(renamed)
  expect(saved.spec.panels.find((item) => item.id === "rps")?.query).toBe(
    savedQuery
  )
})
