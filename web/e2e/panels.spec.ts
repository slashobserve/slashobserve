import type { APIRequestContext, Locator } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const rpsTitle = "Shop requests per second"

async function createOverview(request: APIRequestContext, tooltip?: "all") {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E panels")
  if (tooltip) {
    const rps = (spec.panels as Record<string, unknown>[]).find(
      (panel) => panel.id === "rps"
    )
    rps!.display = { tooltip }
  }
  const { id } = await createDashboard(request, spec)
  return id
}

function legendButton(article: Locator, name: string) {
  return article.getByRole("button", { name, exact: true })
}

test("draws every panel type with data", async ({ page, request }) => {
  const id = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const rps = panel(page, rpsTitle)
  await expect(rps.locator("canvas")).toHaveCount(1)
  await expect(legendButton(rps, "checkout")).toBeVisible()
  await expect(legendButton(rps, "payment")).toBeVisible()

  await expect(panel(page, "Checkout error rate").getByText(/%$/)).toBeVisible()

  const meters = panel(page, "Requests by service").getByRole("meter")
  await expect(meters).toHaveCount(2)
  for (let index = 0; index < 2; index++) {
    await expect(meters.nth(index)).toHaveAttribute("aria-valuetext", /\S/)
  }

  const logs = panel(page, "Log rows by service")
  await expect(logs.getByRole("row").nth(1)).toBeVisible()
})
