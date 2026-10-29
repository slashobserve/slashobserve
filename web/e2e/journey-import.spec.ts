import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { Page } from "@playwright/test"

import { card, entries, entryWith } from "./activity"
import { panel } from "./dashboards"
import { expect, test } from "./fixtures"

const agentTimeout = 90_000
const repoRoot = join(import.meta.dirname, "..", "..")
const demoShopPath = join(
  repoRoot,
  "pkg",
  "grafana",
  "testdata",
  "corpus",
  "slashobserve",
  "demo-shop.json"
)
const unverifiedPath = join(
  repoRoot,
  "e2e",
  "fixtures",
  "api",
  "grafana-unverified.json"
)
const demoShopTitle = "Demo shop (Grafana)"
const demoShopSummary = "27 panels in 6 rows"
const dashboardPath = /\/dashboards\/([0-9a-f-]{36})$/
const fixPrompt = 'Fix the unverified panel "Missing" (p2).'
const missingMetric = "slashobserve_e2e_missing_metric_total"

function readDemoShop(): Record<string, unknown> {
  return JSON.parse(readFileSync(demoShopPath, { encoding: "utf8" })) as Record<
    string,
    unknown
  >
}

function grafanaForms(): {
  name: string
  json: string
  unmatched?: RegExp
}[] {
  const model = readDemoShop()
  const asFile = JSON.stringify(model)
  const wrapped = asFile.replaceAll(
    '"uid":"prometheus"',
    '"uid":"${DS_PROMETHEUS}"'
  )
  expect(wrapped).not.toBe(asFile)
  return [
    { name: "the file as exported", json: asFile },
    {
      name: "an export wrapper",
      unmatched: /DS_PROMETHEUS/,
      json: JSON.stringify({
        ...JSON.parse(wrapped),
        __inputs: [
          {
            name: "DS_PROMETHEUS",
            label: "Prometheus",
            type: "datasource",
            pluginId: "prometheus",
          },
        ],
      }),
    },
    {
      name: "a dashboard envelope",
      json: JSON.stringify({ dashboard: model, meta: {} }),
    },
  ]
}

async function expectPanelsSettled(page: Page) {
  const panels = page.locator("article[data-panel-id]")
  await panels.first().waitFor({ state: "attached" })
  const count = await panels.count()
  expect(count).toBeGreaterThan(0)
  for (let index = 0; index < count; index++) {
    const item = panels.nth(index)
    await item.scrollIntoViewIfNeeded()
    await expect(item).not.toHaveAttribute("data-state", "loading", {
      timeout: 30_000,
    })
  }
}

function importButton(page: Page) {
  return page.getByRole("button", { name: "Import as new dashboard" })
}

async function chooseFile(page: Page, path: string) {
  await page.getByLabel("Grafana dashboard file").setInputFiles(path)
}

async function importedDashboardId(page: Page): Promise<string> {
  const link = page.getByRole("link", { name: "Open dashboard" })
  await expect(link).toBeVisible()
  const href = (await link.getAttribute("href")) ?? ""
  const id = /\/dashboards\/([0-9a-f-]{36})$/.exec(href)?.[1] ?? ""
  expect(id).not.toBe("")
  return id
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("imports a Grafana dashboard from the chat", async ({ page }) => {
      test.setTimeout(120_000)
      await page.route("**/api/v1/news?*", (route) =>
        route.fulfill({
          json: {
            title: "Grafana blog",
            items: [],
            fetched_at: new Date().toISOString(),
          },
        })
      )

      await test.step("start from the composer", async () => {
        await page.goto("/")
        await page.getByRole("link", { name: "Import from Grafana" }).click()
        await expect(page).toHaveURL(/\/import$/)
        await expect(
          page.getByRole("heading", { level: 1, name: "Import from Grafana" })
        ).toBeVisible()
      })

      await test.step("read the file and match datasources", async () => {
        await chooseFile(page, demoShopPath)
        await expect(page.getByText(demoShopSummary)).toBeVisible()
        await expect(
          page.getByRole("combobox", { name: "Datasource for prometheus" })
        ).toContainText("prom (Prometheus)")
        await expect(
          page.getByRole("combobox", { name: "Datasource for clickhouse" })
        ).toContainText("ch (ClickHouse)")
        await expect(importButton(page)).toBeEnabled()
      })

      await test.step("read the report", async () => {
        const report = page.getByRole("region", { name: "Import report" })
        await report.getByRole("tab", { name: /^Placeholders/ }).click()
        const placeholders = report.getByRole("tabpanel").getByRole("listitem")
        await expect(placeholders).toHaveCount(2)
        await expect(placeholders.first()).toContainText("Placeholder")
        await report.getByRole("tab", { name: /^All panels/ }).click()
        await expect(
          report.getByRole("tabpanel").getByRole("listitem")
        ).toHaveCount(27)
      })

      let dashboardId = ""
      await test.step("import", async () => {
        await importButton(page).click()
        await expect(
          page
            .getByRole("status")
            .filter({ hasText: `Imported as ${demoShopTitle}, version 1` })
        ).toBeVisible()
        dashboardId = await importedDashboardId(page)
      })

      await test.step("open the dashboard", async () => {
        await page.getByRole("link", { name: "Open dashboard" }).click()
        await expect(page).toHaveURL(new RegExp(`/dashboards/${dashboardId}$`))
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(
          demoShopTitle
        )
        await expectPanelsSettled(page)
        await expect(page.locator('article[data-state="error"]')).toHaveCount(0)
        const placeholder = panel(page, "Alert groups")
        await expect(placeholder).toHaveAttribute("data-state", "ready")
        await expect(placeholder).toContainText(
          "Grafana panel type: alertGroups"
        )
      })

      await test.step("find it in History", async () => {
        await page.goto("/")
        await page.getByRole("button", { name: "History", exact: true }).click()
        const history = page.getByRole("complementary", { name: "History" })
        await history.getByRole("tab", { name: "Imports" }).click()
        const first = history.getByRole("list").getByRole("link").first()
        await expect(first).toContainText(demoShopTitle)
        await expect(first).toHaveAttribute(
          "href",
          new RegExp(`/dashboards/${dashboardId}$`)
        )
      })

      await test.step("theme", async () => {
        await expect(page.locator("html")).toHaveClass(
          colorScheme === "dark" ? /dark/ : /^(?!.*dark)/
        )
      })
    })

    test("fixes an unverified panel with the agent", async ({
      page,
      request,
    }) => {
      test.setTimeout(180_000)

      await test.step("import the dashboard with a failing panel", async () => {
        await page.goto("/import")
        await chooseFile(page, unverifiedPath)
        await expect(page.getByText("2 panels")).toBeVisible()
        await expect(importButton(page)).toBeEnabled()
        await importButton(page).click()
        await expect(
          page
            .getByRole("status")
            .filter({ hasText: /^Imported as .*, version 1/ })
        ).toBeVisible()

        const report = page.getByRole("region", { name: "Import report" })
        await report.getByRole("tab", { name: /^All panels/ }).click()
        const lines = report.getByRole("tabpanel").getByRole("listitem")
        const missing = lines.filter({ hasText: "Missing" })
        await expect(missing).toContainText("Unverified")
        await expect(missing).toContainText(/no data/i)
        await expect(lines.filter({ hasText: "Up" })).toContainText("Imported")
      })

      let dashboardId = ""
      await test.step("open it", async () => {
        dashboardId = await importedDashboardId(page)
        await page.getByRole("link", { name: "Open dashboard" }).click()
        await expect(page).toHaveURL(dashboardPath)
        await expectPanelsSettled(page)
        await expect(
          panel(page, "Missing").locator("[data-unverified]")
        ).toBeVisible()
      })

      await test.step("ask the agent to fix it", async () => {
        const importEntry = entryWith(
          page,
          "imported this dashboard from Grafana"
        )
        await expect(importEntry).toContainText("1 unverified")
        await expect(
          importEntry.getByRole("button", { name: "Missing" })
        ).toBeVisible()
        await importEntry
          .getByRole("button", { name: "Ask the agent to fix" })
          .click()

        const form = card(page).getByRole("form", {
          name: "Add to the activity",
        })
        await expect(
          form.getByRole("textbox", { name: "Comment or request" })
        ).toHaveValue(fixPrompt)
        await form.getByRole("button", { name: "Send to agent" }).click()
      })

      await test.step("see version 2", async () => {
        const version = entries(page).filter({
          has: page.getByText("Version 2", { exact: true }),
        })
        await expect(version).toHaveCount(1, { timeout: agentTimeout })
        await expect(
          panel(page, "Missing").locator("[data-unverified]")
        ).toHaveCount(0)
        await expect(panel(page, "Missing")).toHaveAttribute(
          "data-state",
          "ready",
          { timeout: 30_000 }
        )
        await expect(
          entryWith(page, "imported this dashboard from Grafana")
        ).toContainText("No panel is unverified in the latest version.")

        await version.getByRole("link", { name: /See changes/ }).click()
        await expect(page).toHaveURL(
          new RegExp(`/dashboards/${dashboardId}/versions`)
        )
        await expect(
          page.getByRole("heading", { level: 2, name: "Changes from v1 to v2" })
        ).toBeVisible()
        const changes = page
          .getByRole("region", { name: 'Panel "Missing"' })
          .getByRole("listitem")
        const edited = changes.filter({ hasText: "queries.A.query" })
        await expect(
          edited
            .getByRole("group", { name: "After", exact: true })
            .getByRole("group", { name: "Query A" })
        ).toContainText("sum(up)")
      })

      await test.step("version 1 stays the faithful import", async () => {
        const response = await request.get(
          `/api/v1/dashboards/${dashboardId}/versions/1`
        )
        expect(response.ok()).toBe(true)
        const body = (await response.json()) as {
          version: {
            spec: { panels: { id: string; queries: { query: string }[] }[] }
          }
        }
        const missing = body.version.spec.panels.find(
          (item) => item.id === "p2"
        )
        expect(missing?.queries[0]?.query).toContain(missingMetric)
      })
    })

    test("accepts every Grafana JSON form", async ({ page }) => {
      for (const form of grafanaForms()) {
        await test.step(form.name, async () => {
          await page.goto("/import")
          await page.getByRole("button", { name: "Paste JSON" }).click()
          await page
            .getByRole("textbox", { name: "Grafana dashboard JSON" })
            .fill(form.json)
          await page.getByRole("button", { name: "Preview" }).click()
          await expect(page.getByText(demoShopSummary)).toBeVisible()
          if (form.unmatched) {
            const choice = page.getByRole("combobox", {
              name: form.unmatched,
            })
            await expect(choice).toContainText("Choose a datasource")
            await expect(importButton(page)).toBeDisabled()
            await choice.click()
            await page
              .getByRole("option", { name: "prom (Prometheus)" })
              .click()
          }
          await expect(importButton(page)).toBeEnabled()
        })
      }
    })
  })
}
