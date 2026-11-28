import type { Page } from "@playwright/test"

import { expect, test } from "./fixtures"

function sidebar(page: Page) {
  return page.getByRole("navigation", { name: "Main" })
}

const pages = [
  ["/agents", "Agents", "Agents"],
  ["/dashboards", "Dashboards", "Dashboards"],
  ["/datasources", "Datasources", "Datasources"],
  ["/keys", "API keys", "API keys"],
  ["/people", "People", "People"],
  ["/settings", "Settings", "Settings"],
] as const

test("renders every shell page with its heading and no crumb repeating it", async ({
  page,
}) => {
  for (const [path, heading, current] of pages) {
    await page.goto(path)
    await expect(
      page.getByRole("heading", { level: 1, name: heading })
    ).toBeVisible()
    await expect(
      page.getByRole("navigation", { name: "Breadcrumb" })
    ).toHaveCount(0)
    const active = sidebar(page).locator('a[aria-current="page"]')
    await expect(active).toHaveCount(1)
    await expect(active).toHaveAccessibleName(new RegExp(`^${current}`))
  }
})
