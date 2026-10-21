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
  return page.getByRole("alertdialog", { name: "Discard your changes?" })
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
    sheet(page).getByRole("textbox", { name: "Query A" })
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

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

  await replaceQuery(page, "sum(rate(")
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  await expect(
    sheet(page)
      .getByRole("group", { name: "Query A" })
      .getByRole("alert")
      .filter({ hasText: "parse error: unclosed left parenthesis" })
  ).toBeVisible()
  await expect(sheet(page).getByText("This panel was rejected:")).toHaveCount(0)
  await expect(sheet(page)).toBeVisible()
  expect((await getDashboard(request, id)).latestVersion).toBe(1)
  expectLoggedOnce(consoleErrors, 422)
})

test("rejects a query that returns no data as a problem of the panel", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await replaceQuery(page, 'up{job="does-not-exist"}')
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  const rejected = sheet(page)
    .getByRole("alert")
    .filter({ hasText: "This panel was rejected:" })
  await expect(rejected).toContainText(
    "query returned no data for the dashboard's default time range and variable values"
  )
  await expect(
    sheet(page).getByRole("group", { name: "Query A" }).getByRole("alert")
  ).toHaveCount(0)
  expect((await getDashboard(request, id)).latestVersion).toBe(1)
  expectLoggedOnce(consoleErrors, 422)
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

test("scopes the activity card from an old comments link", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await page.goto(`/dashboards/${id}?panel=rps&tab=comments`)

  const activity = page.getByRole("complementary", { name: "Activity" })
  await expect(activity).toBeVisible()
  await expect(
    activity.getByRole("combobox", { name: "Panel", exact: true })
  ).toHaveValue("rps")
  await expect(page).toHaveURL(/panel=rps/)
  await expect(page).not.toHaveURL(/tab=|edit=/)
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
    sheet(page).getByText("Open the latest version to edit this panel.")
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
    .getByRole("button", { name: "Apply my changes to version 2" })
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
    .getByRole("button", { name: "Apply my changes to version 2" })
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
    .getByRole("button", { name: "Apply my changes to version 2" })
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

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
    .getByRole("button", { name: "Apply my changes to version 2" })
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
    sheet(page).getByText("Open the latest version to edit this panel.")
  ).toHaveCount(0)

  await sheet(page)
    .getByRole("button", { name: "Apply my changes to version 2" })
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
    sheet(page).getByText("Open the latest version to edit this panel.")
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
  issues: Array<{ path: string; code: string; message: string }>,
  held?: Promise<void>
) {
  return page.route(/\/versions$/, async (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    await held
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  await expect(
    sheet(page).getByText("The panel could not be checked right now.")
  ).toHaveCount(1)
  await expect(sheet(page).getByText("This panel was rejected:")).toHaveCount(0)
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

test("retries a transient failure with the edits made after it", async ({
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
  ])
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()
  await expect(
    sheet(page).getByText("The panel could not be checked right now.")
  ).toBeVisible()
  expectLoggedOnce(consoleErrors, 422)
  await page.unroute(/\/versions$/)

  await sheet(page)
    .getByRole("textbox", { name: "Title" })
    .fill("Edited after the failure")
  await sheet(page).getByRole("button", { name: "Try again" }).click()

  await expect(page.getByText("Saved version 2")).toBeVisible()
  const rps = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "rps"
  )
  expect(rps?.title).toBe("Edited after the failure")
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  await expect(
    sheet(page).getByRole("alert").filter({ hasText: "parse error at char 3" })
  ).toHaveCount(1)
  const others = sheet(page)
    .getByRole("alert")
    .filter({ hasText: "Other problems in this dashboard:" })
  await expect(others.getByRole("listitem")).toHaveText([
    "/panels/1/queries/0/query: unknown metric",
    "A datasource this panel uses is not configured anymore.",
  ])
  await expect(
    sheet(page).getByText("The panel could not be checked right now.")
  ).toHaveCount(1)
  await expect(
    others.getByRole("listitem").filter({ hasText: "could not be checked" })
  ).toHaveCount(0)
  await expect(sheet(page).getByText("query timed out")).toHaveCount(0)
  await expect(
    sheet(page).getByRole("button", { name: "Try again" })
  ).toBeVisible()
  expectLoggedOnce(consoleErrors, 422)
})

test("keeps a rejected query's message on its own row when another row is removed", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await addSecondQuery(page)
  await page.keyboard.insertText("count(up)")

  await mockSaveResponse(page, [
    {
      path: "/panels/0/queries/1/query",
      code: "query_invalid",
      message: "parse error at char 3",
    },
  ])
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()
  const rowB = sheet(page).getByRole("group", { name: "Query B" })
  await expect(rowB.getByRole("alert")).toHaveText("parse error at char 3")
  expectLoggedOnce(consoleErrors, 422)

  await sheet(page).getByRole("button", { name: "Remove query A" }).click()
  await expect(sheet(page).getByRole("group", { name: "Query A" })).toHaveCount(
    0
  )
  await expect(rowB.getByRole("alert")).toHaveText("parse error at char 3")
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()
  await expect(
    sheet(page).getByText("The panel could not be checked right now.")
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
  await sheet(page).getByRole("button", { name: "Add a note" }).click()
  const note = sheet(page).getByRole("textbox", {
    name: "Note for the timeline",
  })
  await note.focus()

  await addVersion(request, id, { ...spec, title: `${spec.title} renamed` }, 1)
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await expect(note).toBeFocused()
})

test("asks before Edit layout drops an unsaved query draft", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await page.setViewportSize({ width: 1700, height: 900 })
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  const title = sheet(page).getByRole("textbox", { name: "Title" })
  await title.fill("Renamed before the layout edit")

  const saveLayout = page.getByRole("button", { name: "Save layout" })
  await page.getByRole("button", { name: /^More/ }).click()
  await page.getByRole("menuitem", { name: "Edit layout" }).click()
  await expect(discardDialog(page)).toBeVisible()
  await expect(saveLayout).toHaveCount(0)

  await page.getByRole("button", { name: "Keep editing" }).click()
  await expect(discardDialog(page)).toHaveCount(0)
  await expect(saveLayout).toHaveCount(0)
  await expect(page).toHaveURL(/panel=rps&edit=query/)
  await expect(title).toHaveValue("Renamed before the layout edit")

  await page.getByRole("button", { name: /^More/ }).click()
  await page.getByRole("menuitem", { name: "Edit layout" }).click()
  await expect(discardDialog(page)).toBeVisible()
  await page.getByRole("button", { name: "Discard" }).click()
  await expect(saveLayout).toBeVisible()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page).not.toHaveURL(/edit=query/)
})

test("asks before Back drops a draft made after opening the latest version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, withRps(spec, { title: "Renamed in v2" }), 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&edit=query`)
  await expect(
    sheet(page).getByText("Open the latest version to edit this panel.")
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
    sheet(page).getByText("Open the latest version to edit this panel.")
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
    sheet(page).getByText("Open the latest version to edit this panel.")
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
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()
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

test("moves between Editor and JSON with the arrow keys and keeps focus on the switch", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  const view = sheet(page).getByRole("radiogroup", { name: "Editor view" })
  const editorRadio = view.getByRole("radio", { name: "Editor" })
  const jsonRadio = view.getByRole("radio", { name: "JSON" })
  await editorRadio.focus()

  await page.keyboard.press("ArrowRight")
  await expect(page).toHaveURL(/panel=rps&edit=json/)
  await expect(jsonRadio).toBeChecked()
  await expect(jsonRadio).toBeFocused()

  await page.keyboard.press("ArrowLeft")
  await expect(page).toHaveURL(/panel=rps&edit=query/)
  await expect(editorRadio).toBeChecked()
  await expect(sheet(page).locator(".cm-content")).toBeVisible()
  await expect(editorRadio).toBeFocused()
})

function queryEditor(page: Page, ref: string) {
  return sheet(page).getByRole("textbox", { name: `Query ${ref}` })
}

async function addSecondQuery(page: Page) {
  await sheet(page).getByRole("button", { name: "Add query" }).click()
  await expect(queryEditor(page, "B")).toBeFocused()
}

test("moves focus to the previous query or Add query after removing a query", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await addSecondQuery(page)
  await sheet(page).getByRole("button", { name: "Remove query B" }).click()
  await expect(queryEditor(page, "B")).toHaveCount(0)
  await expect(queryEditor(page, "A")).toBeFocused()

  await addSecondQuery(page)
  await sheet(page).getByRole("button", { name: "Remove query A" }).click()
  await expect(queryEditor(page, "A")).toHaveCount(0)
  await expect(
    sheet(page).getByRole("button", { name: "Add query" })
  ).toBeFocused()
})

test("does not count whitespace in a legend as a change", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, "Checkout error rate")

  await sheet(page)
    .getByRole("textbox", { name: "Legend for query A" })
    .fill("   ")

  await expect(sheet(page).getByText("No changes")).toBeVisible()
  await expect(sheet(page).getByText("Changed", { exact: true })).toHaveCount(0)
  await expect(
    sheet(page).getByRole("button", { name: saveVersionButton })
  ).toBeDisabled()
})

test("offers Reload page when the panel editor chunk fails to load", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  const chunk = "**/assets/panel-editor-*.js"
  await page.route(chunk, (route) => route.abort())
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)

  await expect(
    sheet(page).getByText("Could not load the panel editor")
  ).toBeVisible()
  await expect(sheet(page).getByRole("button", { name: "Retry" })).toHaveCount(
    0
  )

  await page.unroute(chunk)
  await Promise.all([
    page.waitForEvent("load"),
    sheet(page).getByRole("button", { name: "Reload page" }).click(),
  ])
  await expect(sheet(page).locator(".cm-editor")).toBeVisible()
  const expectedFailure = /net::ERR_FAILED|dynamically imported module/
  consoleErrors.splice(
    0,
    consoleErrors.length,
    ...consoleErrors.filter((text) => !expectedFailure.test(text))
  )
})

test("shows the clash and sends nothing when applying my changes would make the panel invalid", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  const posts: string[] = []
  page.on("request", (message) => {
    if (message.method() === "POST" && /\/versions$/.test(message.url())) {
      posts.push(message.url())
    }
  })
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  await addVersion(
    request,
    id,
    {
      ...spec,
      panels: spec.panels.map((item) =>
        item.id === "rps"
          ? {
              id: "rps",
              type: "text",
              title: rpsTitle,
              content: "Moved to the runbook",
              gridPos: item.gridPos,
            }
          : item
      ),
    },
    1
  )
  await expect(newerAlert(page)).toBeVisible({ timeout: newerTimeout })
  await sheet(page)
    .getByRole("button", { name: "Apply my changes to version 2" })
    .click()

  await expect(
    sheet(page).getByText("Your changes do not fit version 2.")
  ).toBeVisible()
  await expect(sheet(page).locator(".cm-content")).toHaveText(savedQuery)
  expect(posts).toEqual([])
  expect((await getDashboard(request, id)).latestVersion).toBe(2)
})

test("keeps focus on the datasource select when its query editor is rebuilt", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, "SELECT 1")

  const select = sheet(page).getByRole("combobox", {
    name: "Datasource for query A",
  })
  await select.focus()
  await select.selectOption({ label: "ch (ClickHouse)" })

  await expect(queryEditor(page, "A").locator(".tok-keyword")).toHaveText(
    "SELECT"
  )
  await expect(select).toBeFocused()
})

test("edits the content of a text panel and saves it", async ({
  page,
  request,
}) => {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E text")
  spec.panels.push({
    id: "notes",
    type: "text",
    title: "Notes",
    content: "Old notes",
    gridPos: { x: 0, y: 16, w: 12, h: 4 },
  })
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await openFromMenu(page, "Notes")

  await expect(sheet(page).locator(".cm-editor")).toHaveCount(0)
  const content = sheet(page).getByRole("textbox", { name: "Content" })
  await expect(content).toHaveValue("Old notes")
  await content.fill("New **notes**")
  await expect(sheet(page).getByText("1 change: content")).toBeVisible()
  await sheet(page).getByRole("button", { name: saveVersionButton }).click()

  await expect(page.getByText("Saved version 2")).toBeVisible()
  const notes = (await getDashboard(request, id)).spec.panels.find(
    (item) => item.id === "notes"
  )
  expect(notes).toMatchObject({ type: "text", content: "New **notes**" })
})

test("leaves no change behind when a title typed on an untitled text panel is cleared", async ({
  page,
  request,
}) => {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E untitled")
  spec.panels.push({
    id: "notes",
    type: "text",
    content: "Old notes",
    gridPos: { x: 0, y: 16, w: 12, h: 4 },
  })
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await openFromMenu(page, "notes")

  const title = sheet(page).getByRole("textbox", { name: "Title" })
  await title.fill("Temporary")
  await expect(sheet(page).getByText("1 change: title")).toBeVisible()
  await title.fill("")

  await expect(sheet(page).getByText("No changes")).toBeVisible()
  await expect(
    sheet(page).getByRole("button", { name: saveVersionButton })
  ).toBeDisabled()
})

test("keeps focus on Save after a failed save", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await openFromMenu(page, rpsTitle)
  await replaceQuery(page, savedQuery)

  let release = () => {}
  const held = new Promise<void>((resolve) => (release = resolve))
  await mockSaveResponse(
    page,
    [
      {
        path: "/panels/0/title",
        code: "invalid_title",
        message: "the title is rejected",
      },
    ],
    held
  )
  const save = sheet(page).getByRole("button", { name: saveVersionButton })
  await save.click()
  await expect(save).toHaveAttribute("aria-disabled", "true")
  release()

  await expect(sheet(page).getByText("the title is rejected")).toBeVisible()
  expectLoggedOnce(consoleErrors, 422)
  await expect(save).toBeEnabled()
  await expect(save).toBeFocused()
})

test("uses 16px text in a query's datasource select on a narrow screen", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await page.setViewportSize({ width: 700, height: 900 })
  await page.goto(`/dashboards/${id}?panel=rps&edit=query`)

  const select = sheet(page).getByRole("combobox", {
    name: "Datasource for query A",
  })
  await expect(select).toHaveCSS("font-size", "16px")

  await page.setViewportSize({ width: 1200, height: 900 })
  await expect(select).toHaveCSS("font-size", "12px")
})

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("adds a second query and saves it with a note", async ({
      page,
      request,
    }) => {
      const { id, spec } = await createOverview(request)
      await openDashboard(page, id)
      await openFromMenu(page, rpsTitle)

      await addSecondQuery(page)
      await expect(sheet(page).getByText("New", { exact: true })).toBeVisible()
      await page.keyboard.insertText("count(up)")
      await sheet(page)
        .getByRole("textbox", { name: "Legend" })
        .nth(1)
        .fill("up")
      await expect(sheet(page).getByText("1 change: new query B")).toBeVisible()

      await sheet(page).getByRole("button", { name: "Add a note" }).click()
      await sheet(page)
        .getByRole("textbox", { name: "Note for the timeline" })
        .fill("Second query")
      await sheet(page).getByRole("button", { name: saveVersionButton }).click()

      await expect(page.getByText("Saved version 2")).toBeVisible()
      await expect(
        page
          .getByRole("complementary", { name: "Activity" })
          .getByText("Second query")
      ).toBeVisible()
      const rps = (await getDashboard(request, id)).spec.panels.find(
        (item) => item.id === "rps"
      )
      expect(rps?.queries).toMatchObject([
        { query: firstQuery(spec.panels.find((item) => item.id === "rps")) },
        { query: "count(up)", legend: "up" },
      ])
    })

    test("shows lint under the offending line", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openDashboard(page, id)
      await openFromMenu(page, rpsTitle)

      await replaceQuery(page, "sum(rate(up[5m])")
      await expect(sheet(page).locator(".cm-editor .cm-lint-line")).toBeVisible(
        { timeout: 5_000 }
      )
      await expect(
        sheet(page)
          .getByRole("button", { name: /^Error:/ })
          .first()
      ).toBeVisible()
    })

    test("switches to the JSON view through the URL", async ({
      page,
      request,
    }) => {
      const { id } = await createOverview(request)
      await page.goto(`/dashboards/${id}?panel=rps&edit=json`)

      const view = sheet(page).getByRole("radiogroup", { name: "Editor view" })
      await expect(view.getByRole("radio", { name: "JSON" })).toBeChecked()
      await view.getByRole("radio", { name: "Editor" }).click()
      await expect(page).toHaveURL(/panel=rps&edit=query/)
      await expect(view.getByRole("radio", { name: "Editor" })).toBeChecked()
    })

    test("hides Edit for a placeholder panel", async ({ page, request }) => {
      const spec = loadSpec("overview")
      spec.title = uniqueTitle("E2E placeholder")
      spec.panels.push({
        id: "p_ph",
        type: "placeholder",
        title: "Imported",
        reason: "Loki is not supported yet.",
        gridPos: { x: 0, y: 16, w: 12, h: 4 },
      })
      const { id } = await createDashboard(request, spec)
      await openDashboard(page, id)

      await page.getByRole("button", { name: "Panel menu: Imported" }).click()
      await expect(
        page.getByRole("menuitem", { name: "Comment" })
      ).toBeVisible()
      await expect(
        page.getByRole("menuitem", { name: "Edit", exact: true })
      ).toHaveCount(0)
    })

    test("gives each query its own datasource", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openDashboard(page, id)
      await openFromMenu(page, rpsTitle)

      await addSecondQuery(page)
      await page.keyboard.insertText("SELECT 1")
      await expect(queryEditor(page, "B")).toHaveText("SELECT 1")
      await expect(queryEditor(page, "B").locator(".tok-keyword")).toHaveCount(
        0
      )

      await sheet(page)
        .getByRole("combobox", { name: "Datasource for query B" })
        .selectOption({ label: "ch (ClickHouse)" })
      await expect(queryEditor(page, "B").locator(".tok-keyword")).toHaveText(
        "SELECT"
      )
    })
  })
}

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
