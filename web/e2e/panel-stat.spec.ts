import type { APIRequestContext, Locator, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createStats(request: APIRequestContext) {
  const spec = loadSpec("panel-stat")
  spec.title = uniqueTitle("E2E stat panels")
  const { id } = await createDashboard(request, spec)
  return id
}

async function openStats(page: Page, request: APIRequestContext) {
  await openDashboard(page, await createStats(request))
  await expectPanelsReady(page)
}

function tiles(page: Page, title: string): Locator {
  return panel(page, title).locator("[data-stat-tile]")
}

function style(locator: Locator, property: "color" | "backgroundColor") {
  return locator.evaluate(
    (element, name) => getComputedStyle(element)[name],
    property
  )
}

function tokenColor(page: Page, token: string) {
  return page.evaluate((name) => {
    const probe = document.createElement("div")
    probe.style.color = `var(${name})`
    document.body.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  }, token)
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("fills the tile in background mode with readable text", async ({
      page,
      request,
    }) => {
      await openStats(page, request)

      const tile = tiles(page, "Background")
      await expect(tile).toHaveAttribute("data-color-mode", "background")
      expect(await style(tile, "backgroundColor")).not.toBe("rgba(0, 0, 0, 0)")
      expect(["rgb(255, 255, 255)", "rgb(10, 10, 10)"]).toContain(
        await style(tile, "color")
      )
    })

    test("maps a value to text and color", async ({ page, request }) => {
      await openStats(page, request)

      const up = tiles(page, "Mapped").getByText("Up", { exact: true })
      await expect(up).toBeVisible()
      expect(await style(up, "color")).toBe(
        await tokenColor(page, "--threshold-ok")
      )
    })

    test("prints a prefix unit", async ({ page, request }) => {
      await openStats(page, request)

      await expect(tiles(page, "Prefix").getByText(/^\$\d/)).toBeVisible()
    })

    test("lays tiles out horizontally with names", async ({
      page,
      request,
    }) => {
      await openStats(page, request)

      const pods = tiles(page, "Pods")
      await expect
        .poll(() => pods.count(), { message: "one tile per pod" })
        .toBeGreaterThanOrEqual(2)
      const count = await pods.count()
      const tops = new Set<number>()
      for (let index = 0; index < count; index++) {
        const tile = pods.nth(index)
        const box = await tile.boundingBox()
        tops.add(box?.y ?? -1)
        const label = (await tile.getAttribute("aria-label")) ?? ""
        const name = label.split(":")[0]
        expect(name).not.toBe("")
        await expect(tile.getByText(name, { exact: true })).toBeVisible()
      }
      expect(tops.size).toBe(1)
    })

    test("links the value", async ({ page, request }) => {
      await openStats(page, request)

      const link = tiles(page, "Linked").getByRole("link")
      await expect(link).toHaveAttribute("href", /\/dashboards\?q=/)
    })
  })
}
