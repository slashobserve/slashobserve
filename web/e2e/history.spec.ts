import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { openVersionsFromMenu, versionButton } from "./dashboards"
import { expect, test } from "./fixtures"

const simplifiedQuery =
  'sum(rate(traces_span_metrics_calls_total{service_name="checkout",span_kind="SPAN_KIND_SERVER"}[$__rate_interval]))'

const byServiceQuery =
  'sum by (service_name) (rate(traces_span_metrics_calls_total{service_name=~"checkout|payment",span_kind="SPAN_KIND_SERVER"}[$__rate_interval]))'

async function createHistory(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E history")
  const { id } = await createDashboard(request, spec)
  const renamed = { ...spec, title: `${spec.title} renamed` }
  await addVersion(request, id, renamed, 1, "Rename")
  const panels = renamed.panels.map((panel) =>
    panel.id === "rps"
      ? {
          ...panel,
          queries: [
            { query: simplifiedQuery },
            { query: byServiceQuery, legend: "per service" },
          ],
        }
      : panel
  )
  const simplified = { ...renamed, panels }
  await addVersion(request, id, simplified, 2, "Simplify query")
  return { id, spec, renamed, simplified }
}

async function openHistory(page: Page, id: string, search = "") {
  await page.goto(`/d/${id}/versions${search}`)
  await expect(
    page.getByRole("heading", { level: 1, name: /^Versions of / })
  ).toBeVisible()
  await expect(versionsGrid(page)).toBeVisible()
}

function dialog(page: Page) {
  return page.getByRole("alertdialog")
}

function versionsGrid(page: Page) {
  return page.getByRole("grid", { name: "Versions" })
}

function versionRowAt(page: Page, index: number) {
  return versionsGrid(page).locator(`[role="row"][row-index="${index}"]`)
}

test("shows the Spec diff between two versions", async ({ page, request }) => {
  const { id, spec, renamed } = await createHistory(request)
  await openHistory(page, id)

  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v2 to v3" })
  ).toBeVisible()
  const panelGroup = page.getByRole("region", {
    name: 'Panel "Shop requests per second"',
  })
  const changes = panelGroup.getByRole("listitem")

  const edited = changes.filter({ hasText: "queries.A.query" })
  await expect(edited.getByText("Changed")).toBeVisible()
  const before = edited.getByRole("group", { name: "Before", exact: true })
  const after = edited.getByRole("group", { name: "After", exact: true })
  await expect(before.getByRole("group", { name: "Query A" })).toContainText(
    "sum by (service_name)"
  )
  await expect(before).not.toContainText('"query":')
  await expect(after.getByRole("group", { name: "Query A" })).toContainText(
    simplifiedQuery
  )
  await expect(after).not.toContainText('"query":')

  const legend = changes.filter({ hasText: "queries.A.legend" })
  await expect(legend.getByText("Removed")).toBeVisible()
  await expect(
    legend.getByRole("group", { name: "Before", exact: true })
  ).toContainText("{{service_name}}")

  const added = changes.filter({
    has: page.getByText("queries.B", { exact: true }),
  })
  await expect(added.getByText("Added")).toBeVisible()
  const addedAfter = added.getByRole("group", { name: "After", exact: true })
  await expect(
    addedAfter.getByRole("group", { name: "Query B" })
  ).toContainText(byServiceQuery)
  await expect(addedAfter.getByText("legend: per service")).toBeVisible()
  await expect(addedAfter).not.toContainText('"query":')

  await page.getByRole("combobox", { name: "From" }).click()
  await page.getByRole("option", { name: "v1", exact: true }).click()
  await expect(page).toHaveURL(/from=1/)
  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v1 to v3" })
  ).toBeVisible()
  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v1 to v3" })
  ).not.toBeFocused()
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
  await expect(page).toHaveURL(new RegExp(`/d/${id}(/[a-z0-9-]+)?$`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expect(versionButton(page)).toHaveText(/v4$/)

  await openVersionsFromMenu(page)
  const row = versionRowAt(page, 0)
  await expect(row).toContainText("v4")
  await expect(row).toContainText("Restored from v1")
  await expect(row).toContainText("Restore version 1")
})
