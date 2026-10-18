import type { APIRequestContext, Page } from "@playwright/test"

import {
  addVersion,
  createDashboard,
  getDashboard,
  loadSpec,
  uniqueTitle,
  type Spec,
} from "./api"
import { openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const selectAll = process.platform === "darwin" ? "Meta+A" : "Control+A"
const rpsTitle = "Checkout requests per second"
const savedQuery =
  'sum(rate(http_requests_total{service="checkout"}[$__rate_interval]))'
const newerNotice = "Version 2 was saved while you were editing."
const newerTimeout = 10_000
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
  return page.getByRole("dialog")
}

async function openFromMenu(page: Page, title: string) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Edit query" }).click()
}

function stallEvents(page: Page) {
  return page.route(/\/api\/v1\/dashboards\/[^/]+\/events$/, () => undefined)
}

function withRps(spec: Spec, change: Record<string, unknown>): Spec {
  return {
    ...spec,
    panels: spec.panels.map((item) =>
      item.id === "rps" ? { ...item, ...change } : item
    ),
  }
}

function newerAlert(page: Page) {
  return sheet(page).getByRole("alert").filter({ hasText: newerNotice })
}

function discardDialog(page: Page) {
  return page.getByRole("alertdialog", { name: "Discard your query changes?" })
}

function expectLoggedOnce(consoleErrors: string[], status: number) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(logged[0]), 1)
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
  await expect(sheet(page).locator(".cm-content")).toBeFocused()
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
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  const content = sheet(page).locator(".cm-content")
  await expect(content).toHaveText(savedQuery)
  await expect(content).toBeFocused()

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

test("applying my query keeps a panel title renamed in the newer version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()

  await expect(page.getByText("Saved version 3")).toBeVisible()
  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(3)
  const rps = saved.spec.panels.find((item) => item.id === "rps")
  expect(rps?.title).toBe("Renamed in v2")
  expect(rps?.query).toBe(savedQuery)
})

test("applying my title keeps a panel query changed in the newer version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await sheet(page).getByRole("textbox", { name: "Title" }).fill("My title")

  const newerQuery = 'sum(rate(http_requests_total{service="checkout"}[1m]))'
  await addVersion(request, id, withRps(spec, { query: newerQuery }), 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()

  await expect(page.getByText("Saved version 3")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(rps?.title).toBe("My title")
  expect(rps?.query).toBe(newerQuery)
})

test("moves focus to the conflict notice when a save hits a newer version", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await stallEvents(page)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await addVersion(request, id, { ...spec, title: `${spec.title} renamed` }, 1)
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(newerAlert(page)).toBeVisible()
  await expect(newerAlert(page)).toBeFocused()
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)
  expectLoggedOnce(consoleErrors, 409)
})

test("keeps an unsaved query when its panel is deleted in a newer version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  const without = {
    ...spec,
    panels: spec.panels.filter((item) => item.id !== "rps"),
  }
  await addVersion(request, id, without, 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)
  await expect(
    sheet(page).getByRole("heading", { name: rpsTitle })
  ).toBeVisible()

  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()
  await expect(
    sheet(page).getByText("This panel no longer exists in the latest version.")
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)
  expect((await getDashboard(request, id)).latestVersion).toBe(2)
})

test("keeps editing a draft opened on the latest version pinned when a newer one arrives", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&tab=query`)
  const content = sheet(page).locator(".cm-content")
  await expect(content).toHaveAttribute("contenteditable", "true")
  await replaceQuery(page, savedQuery)

  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await expect(content).toHaveAttribute("contenteditable", "true")
  await expect(content).toHaveText(savedQuery)
  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toHaveCount(0)

  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()
  await expect(page.getByText("Saved version 3")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(rps?.query).toBe(savedQuery)
  expect(rps?.title).toBe("Renamed in v2")
})

test("turns read-only when a newer version arrives while nothing is edited", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&tab=query`)
  const content = sheet(page).locator(".cm-content")
  await expect(content).toHaveAttribute("contenteditable", "true")

  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toBeVisible({ timeout: newerTimeout })
  await expect(content).toHaveAttribute("contenteditable", "false")

  await sheet(page).getByRole("button", { name: "Open latest version" }).click()
  await expect(page).not.toHaveURL(/version=/)
  await expect(content).toHaveAttribute("contenteditable", "true")
})

test("asks before browser Back leaves an unsaved query", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await page.goBack()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(page).toHaveURL(/panel=rps&tab=query/)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.goBack()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=|tab=/)
  await expect(page.locator('[data-panel-menu="rps"]')).toBeFocused()
})

test("confirms Escape once with an unsaved query and keeps or discards the draft", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)
  await sheet(page).getByRole("textbox", { name: "Title" }).focus()

  await page.keyboard.press("Escape")
  await expect(discardDialog(page)).toBeVisible()
  await expect(sheet(page)).toBeVisible()

  await page.keyboard.press("Escape")
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(sheet(page)).toBeVisible()
  await expect(page).toHaveURL(/panel=rps&tab=query/)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.keyboard.press("Escape")
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.keyboard.press("Escape")
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=|tab=/)
  await expect(page.locator('[data-panel-menu="rps"]')).toBeFocused()
})

test("points a lint error at the right character after multi-byte text", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, 'up{job="é😀"} + * up')
  const mark = sheet(page).locator(".cm-lintRange-error").first()
  await expect(mark).toBeVisible({ timeout: 5_000 })
  await expect(mark).toHaveText("*")
  await expect(
    sheet(page).getByRole("button", {
      name: /^Error:.*\(line 1, column 16\)$/,
    })
  ).toBeVisible()
})

test("explains that a query is too long to lint", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await sheet(page).locator(".cm-content").click()
  await page.keyboard.press(selectAll)
  await page.keyboard.insertText("a".repeat(65_537))
  await expect(
    sheet(page).getByText("This query is too long to lint.")
  ).toBeVisible({ timeout: 10_000 })
  await expect(
    sheet(page).getByText("Lint is unavailable right now.")
  ).toHaveCount(0)
  expectLoggedOnce(consoleErrors, 400)
})

test.describe("with a touch screen", () => {
  test.use({ hasTouch: true, isMobile: true })

  test("does not open the keyboard by focusing the editor", async ({
    page,
    request,
  }) => {
    const { id } = await createOverview(request)
    await openDashboard(page, id)
    await openFromMenu(page, rpsTitle)

    const content = sheet(page).locator(".cm-content")
    await expect(content).toBeVisible()
    await expect(content).not.toBeFocused()
  })
})
