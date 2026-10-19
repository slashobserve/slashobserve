import type { Route } from "@playwright/test"

import { expect, failedResourceLogged, test, type Page } from "./fixtures"

const isList = (url: URL) => url.pathname === "/api/v1/datasources"

function datasourceRow(page: Page, uid: string) {
  return page.locator(`tr[data-uid="${uid}"]`)
}

function card(page: Page) {
  return page.getByRole("region", { name: "Datasources" })
}

async function openDatasources(page: Page) {
  await page.goto("/datasources")
  await expect(
    page.getByRole("heading", { level: 1, name: "Datasources" })
  ).toBeVisible()
}

async function mockList(page: Page, datasources: unknown[]) {
  await page.route(isList, (route) => route.fulfill({ json: { datasources } }))
}

test("lists the configured datasources", async ({ page }) => {
  await openDatasources(page)

  for (const uid of ["prom", "ch", "prom_tight", "ch_tight"]) {
    await expect(datasourceRow(page, uid)).toBeVisible()
  }

  const prom = datasourceRow(page, "prom")
  await expect(prom).toContainText("Prometheus")
  await expect(prom.locator('[data-status="ok"]')).toHaveText("Healthy")
  await expect(prom.locator("td").nth(3)).toHaveText(
    /^(No dashboards|1 dashboard|\d+ dashboards)$/
  )
  await expect(prom.locator("td").nth(4).locator("time")).toBeVisible()

  await expect(datasourceRow(page, "ch")).toContainText("ClickHouse, read only")
})

test("never shows connection details", async ({ page }) => {
  await openDatasources(page)
  await expect(datasourceRow(page, "prom")).toBeVisible()
  await expect(
    page.getByText(
      "Addresses, users and secrets stay on the server and never reach the browser."
    )
  ).toBeVisible()

  const text = await page.locator("body").innerText()
  for (const forbidden of [
    "http://",
    "https://",
    "password",
    "9090",
    "8123",
    "slashobserve_ro",
  ]) {
    expect(text).not.toContain(forbidden)
  }
})

test("shows an unavailable datasource", async ({ page }) => {
  await mockList(page, [
    {
      uid: "prom-staging",
      type: "prometheus",
      name: "staging",
      healthy: false,
      message: "connection refused",
      checked_at: new Date().toISOString(),
      dashboard_count: 0,
    },
  ])
  await openDatasources(page)

  const row = datasourceRow(page, "prom-staging")
  await expect(row.locator('[data-status="critical"]')).toHaveText(
    "Unavailable: connection refused"
  )
  await expect(row).toContainText("No dashboards")
})

test("shows empty and error states", async ({ page, consoleErrors }) => {
  const empty = (route: Route) => route.fulfill({ json: { datasources: [] } })
  await page.route(isList, empty)
  await openDatasources(page)
  await expect(
    page.getByRole("heading", { name: "No datasources configured" })
  ).toBeVisible()
  await expect(
    page.getByText("Add them to the datasources file and restart the server.")
  ).toBeVisible()
  await page.unroute(isList, empty)

  const fail = (route: Route) =>
    route.fulfill({
      status: 500,
      json: { error: "internal error", code: "internal" },
    })
  await page.route(isList, fail)
  await page.reload()
  const alert = page.getByRole("alert").filter({
    hasText: "Could not load datasources",
  })
  await expect(alert).toBeVisible()
  failedResourceLogged(consoleErrors, "500")

  await page.unroute(isList, fail)
  await alert.getByRole("button", { name: "Retry" }).click()
  await expect(alert).toBeHidden()
  await expect(datasourceRow(page, "prom")).toBeVisible()
})

test("links to the operator guide", async ({ page }) => {
  await openDatasources(page)
  const link = page.getByRole("link", {
    name: "How to add a datasource (opens in a new tab)",
  })
  await expect(link).toHaveAttribute("target", "_blank")
  await expect(link).toHaveAttribute("href", /#datasources-file$/)
})

test("draws both themes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await openDatasources(page)
  await expect(datasourceRow(page, "prom")).toBeVisible()
  await expect(card(page)).toHaveCSS("background-color", "rgb(255, 255, 255)")
  await expect(card(page)).toHaveCSS("border-top-color", "rgb(228, 228, 231)")

  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  await expect(datasourceRow(page, "prom")).toBeVisible()
  await expect(card(page)).toHaveCSS("background-color", "rgb(17, 17, 19)")
  await expect(card(page)).toHaveCSS("border-top-color", "rgb(42, 42, 46)")
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("fits a phone", async ({ page }) => {
    await openDatasources(page)
    await expect(datasourceRow(page, "prom")).toBeVisible()

    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    ).toBe(true)
    expect(
      await card(page).evaluate(
        (element) => element.scrollWidth > element.clientWidth
      )
    ).toBe(true)
  })
})
