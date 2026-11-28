import type { APIRequestContext } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function createTables(request: APIRequestContext) {
  const spec = loadSpec("panel-table")
  spec.title = uniqueTitle("E2E table panels")
  const { id } = await createDashboard(request, spec)
  return id
}

async function openTables(
  page: Parameters<typeof openDashboard>[0],
  id: string
) {
  await openDashboard(page, id)
  await expectPanelsReady(page)
}

function bodyRows(table: ReturnType<typeof panel>) {
  return table.locator('[role=row][row-id]:not([row-id^="footer:"])')
}

test.use({ colorScheme: "light" })

test("keeps every column of a ClickHouse table", async ({ page, request }) => {
  await openTables(page, await createTables(request))

  const table = panel(page, "Services last seen")
  await expect(table.getByRole("columnheader")).toHaveText([
    "service",
    "last_seen",
    "c",
  ])
  expect(await bodyRows(table).count()).toBeGreaterThanOrEqual(2)
})
