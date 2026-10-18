import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard } from "./dashboards"
import { expect, test } from "./fixtures"

const simplifiedQuery =
  'sum(rate(http_requests_total{service="checkout"}[$__rate_interval]))'

async function createHistory(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E history")
  const { id } = await createDashboard(request, spec)
  const renamed = { ...spec, title: `${spec.title} renamed` }
  await addVersion(request, id, renamed, 1, "Rename")
  const panels = renamed.panels.map((panel) =>
    panel.id === "rps" ? { ...panel, query: simplifiedQuery } : panel
  )
  const simplified = { ...renamed, panels }
  await addVersion(request, id, simplified, 2, "Simplify query")
  return { id, spec, renamed, simplified }
}

async function openHistory(page: Page, id: string, search = "") {
  await page.goto(`/dashboards/${id}/versions${search}`)
  await expect(
    page.getByRole("heading", { level: 1, name: "Version history" })
  ).toBeVisible()
  await expect(page.getByRole("table", { name: "Versions" })).toBeVisible()
}

function dialog(page: Page) {
  return page.getByRole("alertdialog")
}

test("lists versions newest first", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  const me = (await (await request.get("/api/v1/me")).json()) as {
    name: string
  }

  await openDashboard(page, id)
  await page.getByRole("link", { name: "History" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}/versions$`))
  await expect(page).toHaveTitle(
    `History - ${spec.title} renamed - SlashObserve`
  )

  const rows = page.getByRole("table", { name: "Versions" }).getByRole("row")
  await expect(rows).toHaveCount(4)
  await expect(rows.nth(1)).toContainText("v3")
  await expect(rows.nth(1)).toContainText("Latest")
  await expect(rows.nth(1)).toContainText("Simplify query")
  await expect(rows.nth(1)).toContainText(me.name)
  await expect(rows.nth(2)).toContainText("v2")
  await expect(rows.nth(2)).toContainText("Rename")
  await expect(rows.nth(2)).not.toContainText("Latest")
  await expect(rows.nth(3)).toContainText("v1")
  await expect(rows.nth(3)).toContainText(me.name)
  await expect(
    page.getByRole("button", { name: "Restore version 3" })
  ).toHaveCount(0)

  await page.getByRole("link", { name: "View version 2" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}\\?version=2$`))
  await openHistory(page, id)
  await page.getByRole("link", { name: "View version 3" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
})

test("shows the Spec diff between two versions", async ({ page, request }) => {
  const { id, spec, renamed } = await createHistory(request)
  await openHistory(page, id)

  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v2 to v3" })
  ).toBeVisible()
  const panelGroup = page.getByRole("region", {
    name: 'Panel "Checkout requests per second"',
  })
  await expect(panelGroup.getByText("Changed")).toBeVisible()
  await expect(panelGroup.getByText("query", { exact: true })).toBeVisible()
  await expect(panelGroup.getByRole("group", { name: "Before" })).toContainText(
    "sum by (pod)"
  )
  await expect(panelGroup.getByRole("group", { name: "After" })).toContainText(
    simplifiedQuery
  )

  await page.getByRole("combobox", { name: "From" }).click()
  await page.getByRole("option", { name: "v1", exact: true }).click()
  await expect(page).toHaveURL(/from=1/)
  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v1 to v3" })
  ).toBeVisible()
  const settings = page.getByRole("region", { name: "Dashboard settings" })
  await expect(settings.getByText("Changed")).toBeVisible()
  await expect(settings.getByText("title", { exact: true })).toBeVisible()
  await expect(settings.getByRole("group", { name: "Before" })).toHaveText(
    new RegExp(`${spec.title}$`)
  )
  await expect(settings.getByRole("group", { name: "After" })).toHaveText(
    new RegExp(`${renamed.title}$`)
  )
})

test("compares a version with its parent", async ({ page, request }) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)

  await page
    .getByRole("button", { name: "Compare version 2 with version 1" })
    .click()
  await expect(page).toHaveURL(/from=1&to=2/)
  const heading = page.getByRole("heading", {
    level: 2,
    name: "Changes from v1 to v2",
  })
  await expect(heading).toBeFocused()
  await expect(
    page.getByRole("region", { name: "Dashboard settings" })
  ).toBeVisible()
  await expect(page.getByRole("region", { name: /^Panel / })).toHaveCount(0)
})

test("restores v1 from history", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await expect(dialog(page)).toBeVisible()
  await expect(
    dialog(page).getByRole("heading", { name: "Restore version 1?" })
  ).toBeVisible()
  await expect(dialog(page)).toContainText(
    "This saves a new version 4 with the dashboard exactly as it was in version 1. Nothing is deleted."
  )
  await expect(
    dialog(page).getByRole("button", { name: "Cancel" })
  ).toBeFocused()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()

  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expect(
    page.getByRole("main").getByText("v4", { exact: true })
  ).toBeVisible()

  await page.getByRole("link", { name: "History" }).click()
  const row = page
    .getByRole("table", { name: "Versions" })
    .getByRole("row")
    .nth(1)
  await expect(row).toContainText("v4")
  await expect(row).toContainText("Restored from v1")
  await expect(row).toContainText("Restore version 1")
})

test("cancelling the restore dialog changes nothing", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 2" }).click()
  await dialog(page).getByRole("button", { name: "Cancel" }).click()
  await expect(dialog(page)).toHaveCount(0)
  await expect(
    page.getByRole("button", { name: "Restore version 2" })
  ).toBeFocused()
  await expect(
    page.getByRole("table", { name: "Versions" }).getByRole("row")
  ).toHaveCount(4)
})

test("restores from the version banner", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openDashboard(page, id, "?version=1")

  await page.getByRole("button", { name: "Restore this version" }).click()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()

  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).not.toHaveURL(/version=/)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expect(
    page.getByRole("main").getByText("v4", { exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Restore this version" })
  ).toHaveCount(0)
})

test("handles a conflict during restore", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, simplified } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await expect(dialog(page)).toBeVisible()
  await addVersion(
    request,
    id,
    { ...simplified, title: uniqueTitle("E2E history other") },
    3,
    "Concurrent change"
  )

  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(dialog(page)).toContainText("Version 4 was saved meanwhile.")
  await expect(dialog(page)).toContainText("This saves a new version 5")
  consoleErrors.length = 0

  await dialog(page)
    .getByRole("button", { name: "Restore on top of version 4" })
    .click()
  await expect(page.getByText("Restored version 1 as version 5")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
})

test("diffs v1 against an empty dashboard", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openHistory(page, id)

  await page
    .getByRole("button", { name: "Compare version 1 with an empty dashboard" })
    .click()
  await expect(page).toHaveURL(/from=0&to=1/)
  await expect(
    page.getByRole("heading", {
      level: 2,
      name: "Changes from an empty dashboard to v1",
    })
  ).toBeFocused()
  const settings = page.getByRole("region", { name: "Dashboard settings" })
  const changes = settings.getByRole("listitem")
  await expect(changes).toHaveCount(1)
  await expect(changes.first()).toContainText("whole dashboard")
  await expect(changes.first().getByText(/^(Added|Changed)$/)).toBeVisible()
  const after = changes.first().getByRole("group", { name: "After" })
  await after.getByRole("button", { name: "Show all" }).click()
  await expect(after).toContainText(spec.title)
  await expect(after.getByRole("button", { name: "Show all" })).toHaveCount(0)
})
