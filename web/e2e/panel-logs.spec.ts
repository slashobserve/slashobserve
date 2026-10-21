import type { APIRequestContext } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createLogs(request: APIRequestContext) {
  const spec = loadSpec("panel-logs")
  spec.title = uniqueTitle("E2E logs panels")
  const { id } = await createDashboard(request, spec)
  return id
}

async function openLogs(page: Parameters<typeof openDashboard>[0], id: string) {
  await openDashboard(page, id)
  await expectPanelsReady(page)
}

function lines(page: Parameters<typeof panel>[0], title: string) {
  return panel(page, title).locator("ul[data-logs-count] > li")
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("lists log lines with time and level", async ({ page, request }) => {
      await openLogs(page, await createLogs(request))

      const list = panel(page, "Newest logs").locator("ul[data-logs-count]")
      expect(
        Number(await list.getAttribute("data-logs-count"))
      ).toBeGreaterThan(0)
      const first = lines(page, "Newest logs").first()
      await expect(first.locator("time")).toHaveAttribute("datetime", /.+/)
      await expect(first.locator("[data-logs-level]")).toHaveText(/\S/)
      await expect(first).toHaveAttribute(
        "data-level",
        /^(error|warning|info|other)$/
      )
    })

    test("expands a line", async ({ page, request }) => {
      await openLogs(page, await createLogs(request))

      const first = lines(page, "Newest logs").first()
      const toggle = first.getByRole("button")
      await expect(toggle).toHaveAttribute("aria-expanded", "false")
      await toggle.focus()
      await page.keyboard.press("Enter")
      await expect(toggle).toHaveAttribute("aria-expanded", "true")
      await expect(
        first.locator("dl dt", { hasText: "ServiceName" })
      ).toHaveCount(1)
      await page.keyboard.press("Space")
      await expect(toggle).toHaveAttribute("aria-expanded", "false")
    })

    test("orders oldest first", async ({ page, request }) => {
      await openLogs(page, await createLogs(request))

      const rows = lines(page, "Oldest logs")
      expect(await rows.count()).toBeGreaterThan(1)
      const first = await rows.first().locator("time").getAttribute("datetime")
      const last = await rows.last().locator("time").getAttribute("datetime")
      expect(Date.parse(first ?? "")).toBeLessThanOrEqual(
        Date.parse(last ?? "")
      )

      const newest = lines(page, "Newest logs")
      const top = await newest.first().locator("time").getAttribute("datetime")
      const bottom = await newest
        .last()
        .locator("time")
        .getAttribute("datetime")
      expect(Date.parse(top ?? "")).toBeGreaterThanOrEqual(
        Date.parse(bottom ?? "")
      )
    })
  })
}
