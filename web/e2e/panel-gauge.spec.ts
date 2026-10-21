import type { APIRequestContext } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createGauges(request: APIRequestContext) {
  const spec = loadSpec("panel-gauge")
  spec.title = uniqueTitle("E2E gauge panels")
  const { id } = await createDashboard(request, spec)
  return id
}

async function openGauges(
  page: Parameters<typeof openDashboard>[0],
  id: string
) {
  await openDashboard(page, id)
  await expectPanelsReady(page)
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("draws a gauge with threshold markers", async ({ page, request }) => {
      await openGauges(page, await createGauges(request))

      const meters = panel(page, "Error percent").getByRole("meter")
      await expect(meters).toHaveCount(1)
      await expect(meters).toHaveAttribute("aria-valuemin", "0")
      await expect(meters).toHaveAttribute("aria-valuemax", "100")
      await expect(meters).toHaveAttribute("data-markers", "true")
      await expect(meters.getByText(/^-?[\d.,]+%$/)).toBeVisible()
    })

    test("shows the threshold word", async ({ page, request }) => {
      await openGauges(page, await createGauges(request))

      const meter = panel(page, "Error percent").getByRole("meter")
      await expect(meter.getByText(/^(OK|Warning|Critical)$/)).toHaveCount(1)
    })

    test("draws one gauge per series without markers", async ({
      page,
      request,
    }) => {
      await openGauges(page, await createGauges(request))

      const meters = panel(page, "Requests by pod").getByRole("meter")
      await expect.poll(() => meters.count()).toBeGreaterThanOrEqual(2)
      for (const meter of await meters.all()) {
        await expect(meter).toHaveAttribute("data-markers", "false")
        await expect(meter).toHaveAttribute("aria-label", /^[a-z]+-\d+$/)
      }
    })
  })
}
