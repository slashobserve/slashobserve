import type { APIRequestContext, Locator } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createFeatures(request: APIRequestContext) {
  const spec = loadSpec("panel-timeseries")
  spec.title = uniqueTitle("E2E timeseries features")
  const { id } = await createDashboard(request, spec)
  return id
}

function legendOf(article: Locator) {
  return article.getByRole("group", { name: "Series" })
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("applies overrides by query and regex", async ({ page, request }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      const article = panel(page, "Requests and errors")
      await expect(article.locator("[data-axes]")).toHaveAttribute(
        "data-axes",
        /\bright\b/
      )
      const legend = legendOf(article)
      await expect(
        legend.getByRole("button", { name: "total", exact: true })
      ).toBeVisible()
      await expect(
        legend.getByRole("button", { name: /checkout-1/ })
      ).toHaveCount(0)
      await expect(
        legend.getByRole("columnheader", { name: "Mean" })
      ).toBeVisible()
      await expect(
        legend.getByRole("columnheader", { name: "Max" })
      ).toBeVisible()
    })

    test("places the legend on the right", async ({ page, request }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      const article = panel(page, "Requests and errors")
      const canvas = await article.locator("canvas").boundingBox()
      const legend = await legendOf(article).boundingBox()
      expect(canvas).not.toBeNull()
      expect(legend).not.toBeNull()
      expect(legend!.x).toBeGreaterThan(canvas!.x + canvas!.width / 2)
    })

    test("stacks bars", async ({ page, request }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      const article = panel(page, "Stacked bars")
      await expect(article.locator("[data-stack]")).toHaveAttribute(
        "data-stack",
        "normal"
      )
      const legend = legendOf(article)
      await expect(legend.locator('[data-style="bars"]').first()).toBeVisible()
      await expect(
        legend.locator('[data-style]:not([data-style="bars"])')
      ).toHaveCount(0)
    })

    test("stacks to percent", async ({ page, request }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      await expect(
        panel(page, "Percent stack").locator("[data-stack]")
      ).toHaveAttribute("data-stack", "percent")
    })

    test("hides the legend and shows one tooltip row", async ({
      page,
      request,
    }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      const article = panel(page, "Points")
      await expect(legendOf(article)).toHaveCount(0)
      await article.locator(".u-over").hover()
      const tooltip = article.locator('[data-slot="chart-tooltip"]')
      await expect(tooltip).toBeVisible()
      await expect(tooltip.locator(".ts-tooltip-row")).toHaveCount(1)
    })

    test("shows every series in an all tooltip", async ({ page, request }) => {
      await openDashboard(page, await createFeatures(request))
      await expectPanelsReady(page)

      const article = panel(page, "Requests and errors")
      await article.locator(".u-over").hover()
      const tooltip = article.locator('[data-slot="chart-tooltip"]')
      await expect(tooltip).toBeVisible()
      expect(await tooltip.locator(".ts-tooltip-row").count()).toBeGreaterThan(
        1
      )
      await expect(
        tooltip.locator(".ts-tooltip-row", { hasText: "checkout-1" })
      ).toHaveCount(0)
    })
  })
}
