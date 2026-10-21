import type { APIRequestContext, Locator } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createBars(request: APIRequestContext) {
  const spec = loadSpec("panel-bargauge")
  spec.title = uniqueTitle("E2E bar gauge panels")
  const { id } = await createDashboard(request, spec)
  return id
}

async function positions(meters: Locator) {
  const boxes = await Promise.all(
    (await meters.all()).map((meter) => meter.boundingBox())
  )
  return boxes.map((box) => {
    if (!box) throw new Error("a meter has no box")
    return box
  })
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("lays bars out horizontally", async ({ page, request }) => {
      await openDashboard(page, await createBars(request))
      await expectPanelsReady(page)

      const bars = panel(page, "Pods horizontal")
      await expect(bars.getByRole("region")).toHaveAttribute(
        "data-orientation",
        "horizontal"
      )
      const meters = bars.getByRole("meter")
      expect(await meters.count()).toBeGreaterThanOrEqual(2)
      const boxes = await positions(meters)
      expect(new Set(boxes.map((box) => box.y)).size).toBe(boxes.length)
    })

    test("lays bars out vertically with an override color", async ({
      page,
      request,
    }) => {
      await openDashboard(page, await createBars(request))
      await expectPanelsReady(page)

      const bars = panel(page, "Services vertical")
      await expect(bars.getByRole("region")).toHaveAttribute(
        "data-orientation",
        "vertical"
      )
      const meters = bars.getByRole("meter")
      expect(await meters.count()).toBeGreaterThanOrEqual(2)
      const boxes = await positions(meters)
      expect(new Set(boxes.map((box) => box.x)).size).toBe(boxes.length)

      const fill = await meters
        .first()
        .locator('[data-part="fill"]')
        .evaluate((element) => getComputedStyle(element).backgroundColor)
      const info = await page.evaluate(() => {
        const probe = document.createElement("span")
        probe.style.backgroundColor = "var(--threshold-info)"
        document.body.append(probe)
        const color = getComputedStyle(probe).backgroundColor
        probe.remove()
        return color
      })
      expect(fill).toBe(info)
    })
  })
}
