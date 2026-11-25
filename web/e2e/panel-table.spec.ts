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

function firstNumber(text: string): number {
  const match = /-?\d[\d,]*(?:\.\d+)?/.exec(text)
  if (!match) throw new Error(`no number in "${text}"`)
  return Number(match[0].replaceAll(",", ""))
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("applies a table format and transforms", async ({ page, request }) => {
      await openTables(page, await createTables(request))

      const top = panel(page, "Top routes")
      await expect(bodyRows(top)).toHaveCount(3)
      await expect(
        top.getByRole("columnheader", { name: "Requests" })
      ).toBeVisible()
      await expect(top.getByRole("columnheader", { name: "Time" })).toHaveCount(
        0
      )

      const cells = await top
        .locator('[data-column="Requests"]')
        .allTextContents()
      expect(cells).toHaveLength(3)
      expect(firstNumber(cells[0])).toBeGreaterThanOrEqual(
        firstNumber(cells[1])
      )
    })

    test("colors threshold cells with an icon", async ({ page, request }) => {
      await openTables(page, await createTables(request))

      const cells = panel(page, "Top routes").locator(
        '[data-column="Requests"]'
      )
      await expect(cells).toHaveCount(3)
      for (const cell of await cells.all()) {
        await expect(cell).toHaveAttribute("data-threshold", /.+/)
        await expect(cell.locator("svg")).toHaveCount(1)
      }
    })

    test("adds a footer", async ({ page, request }) => {
      await openTables(page, await createTables(request))

      const footer = panel(page, "Top routes")
        .locator("[data-footer-label]")
        .first()
      await expect(footer).toHaveAttribute("data-footer-label", "Total")
      await expect(footer).toContainText("Total")
    })

    test("links cells with row values and hides the header", async ({
      page,
      request,
    }) => {
      await openTables(page, await createTables(request))

      const logs = panel(page, "Log rows")
      const service = logs.getByRole("columnheader", { name: "service" })
      await expect(service).toHaveCount(1)
      await expect(service.getByRole("button")).toHaveCount(0)
      await expect(
        logs.locator('[data-column="service"] a').first()
      ).toHaveAttribute("href", /\/dashboards\?q=.+/)
    })

    test("View as table still works on a timeseries", async ({
      page,
      request,
    }) => {
      const spec = loadSpec("overview")
      spec.title = uniqueTitle("E2E table view")
      const { id } = await createDashboard(request, spec)
      await openTables(page, id)

      const title = "Shop requests per second"
      const rps = panel(page, title)
      await rps.getByRole("button", { name: `Panel menu: ${title}` }).click()
      await page.getByRole("menuitem", { name: "View as table" }).click()
      await expect(
        rps.getByRole("columnheader", { name: "Value" })
      ).toBeVisible()
      await expect(rps.locator('[data-column="Value"]').first()).toContainText(
        "req/s"
      )
      await expect(rps.locator("canvas")).toHaveCount(0)
    })

    test("keeps every column of a ClickHouse table", async ({
      page,
      request,
    }) => {
      await openTables(page, await createTables(request))

      const table = panel(page, "Services last seen")
      await expect(table.getByRole("columnheader")).toHaveText([
        "service",
        "last_seen",
        "c",
      ])
      expect(await bodyRows(table).count()).toBeGreaterThanOrEqual(2)
    })

    test("names reduced rows by the legend", async ({ page, request }) => {
      await openTables(page, await createTables(request))

      const cells = panel(page, "Targets by job").locator(
        '[data-column="Field"]'
      )
      await expect.poll(() => cells.count()).toBeGreaterThan(0)
      for (const name of await cells.allTextContents())
        expect(name).not.toContain("{")
    })
  })
}
