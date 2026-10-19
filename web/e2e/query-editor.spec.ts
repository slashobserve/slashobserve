import type { APIRequestContext, Page } from "@playwright/test"

import {
  addVersion,
  createDashboard,
  getDashboard,
  loadSpec,
  uniqueTitle,
  type Spec,
} from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
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
  return page.getByRole("complementary", { name: /^Edit / })
}

async function openFromMenu(page: Page, title: string) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click()
}

function stallEvents(page: Page) {
  return page.route(/\/api\/v1\/dashboards\/[^/]+\/events$/, () => undefined)
}

function firstQuery(panel: Record<string, unknown> | undefined): unknown {
  return (panel?.queries as Array<{ query: string }> | undefined)?.[0]?.query
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

  await expect(page).toHaveURL(/panel=rps&edit=query/)
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

  await expect(page).toHaveURL(/panel=logs&edit=query/)
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
  await expect(versionButton(page)).toHaveText(/v2$/)

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(2)
  const rps = saved.spec.panels.find((item) => item.id === "rps")
  expect(firstQuery(rps)).toBe(savedQuery)
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
  await expect(page).not.toHaveURL(/panel=|edit=/)
  await expect(page.locator('[data-panel-menu="rps"]')).toBeFocused()
})

test("opens from an old link with the query tab", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await page.goto(`/dashboards/${id}?panel=logs&tab=query`)

  await expect(sheet(page)).toBeVisible()
  await expect(page).toHaveURL(/panel=logs&edit=query/)
  await expect(page).not.toHaveURL(/tab=/)
  await expect(
    sheet(page).getByRole("heading", { name: "Edit Log rows by service" })
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-editor")).toBeVisible()
})

test("opens directly from a link", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await page.goto(`/dashboards/${id}?panel=logs&edit=query`)

  await expect(sheet(page)).toBeVisible()
  await expect(
    sheet(page).getByRole("heading", { name: "Edit Log rows by service" })
  ).toBeVisible()
  const logs = spec.panels.find((item) => item.id === "logs")
  await expect(sheet(page).locator(".cm-content")).toContainText(
    String(firstQuery(logs)).slice(0, 30)
  )
})

test("is read-only on an old version", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)

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
  expect(firstQuery(saved.spec.panels.find((item) => item.id === "rps"))).toBe(
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
  expect(firstQuery(rps)).toBe(savedQuery)
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
  await addVersion(
    request,
    id,
    withRps(spec, { queries: [{ query: newerQuery }] }),
    1
  )
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await sheet(page)
    .getByRole("button", { name: "Apply my query to version 2" })
    .click()

  await expect(page.getByText("Saved version 3")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(rps?.title).toBe("My title")
  expect(firstQuery(rps)).toBe(newerQuery)
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
    sheet(page).getByRole("heading", { name: `Edit ${rpsTitle}` })
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
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)
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
  expect(firstQuery(rps)).toBe(savedQuery)
  expect(rps?.title).toBe("Renamed in v2")
})

test("turns read-only when a newer version arrives while nothing is edited", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)
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
  await expect(page).toHaveURL(/panel=rps&edit=query/)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.goBack()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=|edit=/)
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
  await expect(page).toHaveURL(/panel=rps&edit=query/)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.keyboard.press("Escape")
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.keyboard.press("Escape")
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/panel=|edit=/)
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

function mockSaveResponse(
  page: Page,
  issues: Array<{ path: string; code: string; message: string }>
) {
  return page.route(/\/versions$/, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    return route.fulfill({
      status: 422,
      contentType: "application/json",
      body: JSON.stringify({
        error: "spec is invalid",
        code: "invalid_spec",
        issues,
      }),
    })
  })
}

test("offers Try again when the server cannot check the query right now", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await mockSaveResponse(page, [
    {
      path: "/panels/0/queries/0/query",
      code: "datasource_unavailable",
      message: "prometheus returned 502",
    },
    {
      path: "/panels/3/queries/0/query",
      code: "query_timeout",
      message: "query timed out",
    },
  ])
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(
    sheet(page).getByText("The query could not be checked right now.")
  ).toHaveCount(1)
  await expect(sheet(page).getByText("This query was rejected:")).toHaveCount(0)
  await expect(
    sheet(page).getByText("Other problems in this dashboard:")
  ).toHaveCount(0)
  await expect(sheet(page).getByText("prometheus returned 502")).toHaveCount(0)
  expectLoggedOnce(consoleErrors, 422)

  await page.unroute(/\/versions$/)
  await sheet(page).getByRole("button", { name: "Try again" }).click()
  await expect(page.getByText("Saved version 2")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(firstQuery(rps)).toBe(savedQuery)
})

test("lists each rejected query issue once and the transient line once", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  const own = {
    path: "/panels/0/queries/0/query",
    code: "query_invalid",
    message: "parse error at char 3",
  }
  const other = {
    path: "/panels/1/queries/0/query",
    code: "query_invalid",
    message: "unknown metric",
  }
  const timedOut = (path: string) => ({
    path,
    code: "query_timeout",
    message: "query timed out",
  })
  await mockSaveResponse(page, [
    own,
    own,
    timedOut("/panels/0/queries/0/query"),
    other,
    other,
    timedOut("/panels/2/queries/0/query"),
    {
      path: "/panels/3/queries/0/query",
      code: "unknown_datasource",
      message: 'datasource "gone" is not configured',
    },
  ])
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(sheet(page).getByText("This query was rejected:")).toBeVisible()
  await expect(
    sheet(page)
      .getByRole("alert")
      .filter({ hasText: "This query was rejected:" })
      .getByRole("listitem")
  ).toHaveText([
    "parse error at char 3",
    "/panels/1/queries/0/query: unknown metric",
    "A datasource this query uses is not configured anymore.",
  ])
  await expect(
    sheet(page).getByText("The query could not be checked right now.")
  ).toHaveCount(1)
  await expect(
    sheet(page)
      .getByRole("alert")
      .filter({ hasText: "This query was rejected:" })
      .getByRole("listitem")
      .filter({ hasText: "could not be checked" })
  ).toHaveCount(0)
  await expect(sheet(page).getByText("query timed out")).toHaveCount(0)
  await expect(
    sheet(page).getByRole("button", { name: "Try again" })
  ).toBeVisible()
  expectLoggedOnce(consoleErrors, 422)
})

test("applies to the newer version when Try again follows a transient error", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await mockSaveResponse(page, [
    {
      path: "/panels/0/queries/0/query",
      code: "datasource_unavailable",
      message: "prometheus returned 502",
    },
  ])
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()
  await expect(
    sheet(page).getByText("The query could not be checked right now.")
  ).toBeVisible()
  expectLoggedOnce(consoleErrors, 422)
  await page.unroute(/\/versions$/)

  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await sheet(page).getByRole("button", { name: "Try again" }).click()

  await expect(page.getByText("Saved version 3")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(firstQuery(rps)).toBe(savedQuery)
  expect(rps?.title).toBe("Renamed in v2")
})

test("leaves focus where it is when a newer version arrives inside the sheet", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)
  const summary = sheet(page).getByRole("textbox", { name: "Summary" })
  await summary.focus()

  await addVersion(request, id, { ...spec, title: `${spec.title} renamed` }, 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await expect(summary).toBeFocused()
})

test("asks before Back drops a draft made after opening the latest version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)
  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toBeVisible()
  await sheet(page).getByRole("button", { name: "Open latest version" }).click()
  await expect(page).not.toHaveURL(/version=/)
  await expect(sheet(page).locator(".cm-content")).toHaveAttribute(
    "contenteditable",
    "true"
  )
  await replaceQuery(page, savedQuery)

  await page.goBack()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/version=/)
  await expect(page).toHaveURL(/panel=rps&edit=query/)
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)

  await page.goBack()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(page).toHaveURL(/version=1/)
  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-content")).not.toHaveText(savedQuery)
})

test("focuses the heading when a read-only panel opens", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)

  await expect(
    sheet(page).getByText("Open the latest version to edit this query.")
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-content")).not.toBeFocused()
  await expect(
    sheet(page).getByRole("heading", { name: `Edit ${rpsTitle}` })
  ).toBeFocused()
})

function moveHandle(page: Page, title: string) {
  return page.getByRole("button", { name: `Move or resize ${title}` })
}

async function startLayoutEditing(page: Page) {
  await page.getByRole("button", { name: /^More/ }).click()
  await page.getByRole("menuitem", { name: "Edit layout" }).click()
  await expect(page.getByRole("button", { name: "Save layout" })).toBeVisible()
}

async function nudgeErrorRate(page: Page) {
  await moveHandle(page, "Checkout error rate").focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByText("Unsaved changes")).toBeVisible()
}

test("keeps Edit disabled while the layout is being edited", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  await page.getByRole("button", { name: `Panel menu: ${rpsTitle}` }).click()
  await expect(
    page.getByRole("menuitem", {
      name: "Edit (finish the layout first)",
    })
  ).toHaveAttribute("aria-disabled", "true")
  await page.keyboard.press("Escape")
  await expect(sheet(page)).toHaveCount(0)
})

test("ignores a panel link reached with Back during a layout session", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page, rpsTitle)
  await expect(sheet(page).locator(".cm-editor")).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(sheet(page)).toHaveCount(0)

  await startLayoutEditing(page)
  await nudgeErrorRate(page)
  await page.goBack()

  await expect(page).not.toHaveURL(/panel=|edit=/)
  await expect(sheet(page)).toHaveCount(0)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  await expect(page.getByText("Unsaved changes")).toBeVisible()
})

test("asks once when leaving the dashboard with an unsaved layout", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)
  await nudgeErrorRate(page)

  await versionButton(page).click()
  await page.getByRole("menuitem", { name: "Compare versions" }).click()
  await expect(page.getByRole("alertdialog")).toHaveCount(1)
  await expect(
    page.getByRole("alertdialog", { name: "Discard unsaved layout changes?" })
  ).toBeVisible()
  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}(\\?|$)`))
})

test("saves a query and then a layout on top of it", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()
  await expect(page.getByText("Saved version 2")).toBeVisible()
  await expect(sheet(page)).toHaveCount(0)
  await expectPanelsReady(page)

  await startLayoutEditing(page)
  await nudgeErrorRate(page)
  await page.getByRole("button", { name: "Save layout" }).click()
  await page
    .getByRole("dialog", { name: "Save layout" })
    .getByRole("button", { name: "Save", exact: true })
    .click()
  await expect(page.getByText("Saved version 3")).toBeVisible()

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(3)
  const rps = saved.spec.panels.find((item) => item.id === "rps")
  expect(firstQuery(rps)).toBe(savedQuery)
  const errorRate = saved.spec.panels.find((item) => item.id === "error_rate")
  expect(errorRate?.gridPos).toMatchObject({ x: 13 })
})

test("explains that a request body over the limit is too long to lint", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await sheet(page).locator(".cm-content").click()
  await page.keyboard.press(selectAll)
  await page.keyboard.insertText("a".repeat(600_000))
  await expect(
    sheet(page).getByText("This query is too long to lint.")
  ).toBeVisible({ timeout: 15_000 })
  expectLoggedOnce(consoleErrors, 413)
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
