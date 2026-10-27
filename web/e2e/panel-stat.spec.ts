import type { APIRequestContext, Locator, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard } from "./dashboards"
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
  return page
    .getByRole("article", { name: title, exact: true })
    .locator("[data-stat-tile]")
}

function style(
  locator: Locator,
  property: "color" | "backgroundColor" | "fontSize"
) {
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

function channels(color: string): [number, number, number] {
  const match = /rgba?\((\d+), (\d+), (\d+)/.exec(color)
  if (!match) throw new Error(`not an rgb color: ${color}`)
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

function luminance(color: string) {
  const [red, green, blue] = channels(color).map((channel) => {
    const unit = channel / 255
    return unit <= 0.03928 ? unit / 12.92 : ((unit + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue
}

function contrast(a: string, b: string) {
  const [high, low] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (high + 0.05) / (low + 0.05)
}

test.describe("theme switch", () => {
  test.use({ colorScheme: "light" })

  test("recomputes the text color when the theme flips", async ({
    page,
    request,
  }) => {
    await openStats(page, request)

    const tile = tiles(page, "Info background")
    await expect(tile).toHaveAttribute("data-threshold", "info")
    await expect.poll(() => style(tile, "color")).toBe("rgb(255, 255, 255)")

    await page.getByRole("button", { name: "Switch theme" }).click()
    await expect(page.locator("html")).toHaveClass(/\bdark\b/)

    await expect.poll(() => style(tile, "color")).toBe("rgb(10, 10, 10)")
  })
})

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
      const text = await style(tile, "color")
      expect(["rgb(255, 255, 255)", "rgb(10, 10, 10)"]).toContain(text)
      expect(
        contrast(await style(tile, "backgroundColor"), text)
      ).toBeGreaterThanOrEqual(4.5)
    })

    test("maps a value to text in the foreground color and a colored icon", async ({
      page,
      request,
    }) => {
      await openStats(page, request)

      const mapped = tiles(page, "Mapped")
      const up = mapped.getByText("Up", { exact: true })
      await expect(up).toBeVisible()
      expect(await style(up, "color")).toBe(
        await tokenColor(page, "--foreground")
      )
      expect(await style(mapped.locator("[data-stat-icon]"), "color")).toBe(
        await tokenColor(page, "--threshold-ok")
      )
    })

    test("draws the value large with a muted unit and colors only the icon", async ({
      page,
      request,
    }) => {
      await openStats(page, request)

      const tile = tiles(page, "State")
      await expect(tile).toHaveAttribute("data-threshold", "critical")
      const value = tile.locator("[data-stat-value]")
      const unit = tile.locator("[data-stat-unit]")
      expect(await style(value, "fontSize")).toBe("28px")
      expect(await style(value, "color")).toBe(
        await tokenColor(page, "--foreground")
      )
      expect(await style(unit, "fontSize")).toBe("13px")
      expect(await style(unit, "color")).toBe(
        await tokenColor(page, "--muted-foreground")
      )
      expect(await style(tile.locator("[data-stat-icon]"), "color")).toBe(
        await tokenColor(page, "--threshold-critical")
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

      const link = tiles(page, "Linked").getByRole("link", {
        name: /Details for/,
      })
      await expect(link).toHaveAttribute("href", /\/dashboards\?q=/)
      await expect(link).toHaveAttribute("title", /^Details for /)
    })
  })
}
