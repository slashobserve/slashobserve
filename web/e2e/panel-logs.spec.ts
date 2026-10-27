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
      await expect(first.locator("time")).toHaveText(
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$/
      )
      await expect(first.locator("[data-logs-level]")).toHaveText(/\S/)
      const info = panel(page, "Newest logs").locator(
        'ul[data-logs-count] > li[data-level="info"]'
      )
      await expect(info.first().locator("[data-logs-level]")).toHaveText("Info")
    })

    test("shows normalised level words in full", async ({ page, request }) => {
      await openLogs(page, await createLogs(request))

      const rows = lines(page, "Level words")
      await expect(rows).toHaveCount(12)
      const shown = await rows.evaluateAll((items) =>
        items.map((item) => {
          const word = item.querySelector("[data-logs-level] span.truncate")
          return {
            kind: item.getAttribute("data-level"),
            text: word?.textContent,
            clipped: word ? word.scrollWidth > word.clientWidth : true,
          }
        })
      )
      expect(shown.map(({ kind, text }) => [text, kind])).toEqual([
        ["Critical", "error"],
        ["Critical", "error"],
        ["Critical", "error"],
        ["Error", "error"],
        ["Error", "error"],
        ["Warning", "warning"],
        ["Warning", "warning"],
        ["Info", "info"],
        ["Info", "info"],
        ["Debug", "other"],
        ["Trace", "other"],
        ["Verbose", "other"],
      ])
      expect(shown.filter(({ clipped }) => clipped)).toEqual([])
    })

    test("keeps the line cap note above the scrolling lines", async ({
      page,
      request,
    }) => {
      await openLogs(page, await createLogs(request))

      const card = panel(page, "Capped logs")
      await expect(card.locator("ul[data-logs-count]")).toHaveAttribute(
        "data-logs-count",
        "1000"
      )
      const note = card.getByText("Showing the newest 1,000 lines.")
      await expect(note).toBeVisible()
      const before = await note.boundingBox()
      const list = card.locator("ul[data-logs-count]")
      const listTop = (await list.boundingBox())?.y ?? 0
      expect((before?.y ?? 0) + (before?.height ?? 0)).toBeLessThanOrEqual(
        listTop
      )
      await list.evaluate((el) => {
        el.parentElement?.scrollTo({ top: el.scrollHeight })
      })
      await expect(lines(page, "Capped logs").last()).toBeInViewport()
      expect((await note.boundingBox())?.y).toBe(before?.y)
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
      await expect.poll(() => rows.count()).toBeGreaterThan(1)
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
