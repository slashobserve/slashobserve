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

test.use({ colorScheme: "light" })

test("lists log lines with time and level", async ({ page, request }) => {
  await openLogs(page, await createLogs(request))

  const list = panel(page, "Newest logs").locator("ul[data-logs-count]")
  expect(Number(await list.getAttribute("data-logs-count"))).toBeGreaterThan(0)
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
