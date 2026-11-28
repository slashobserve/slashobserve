import type { APIRequestContext, Locator, Page } from "@playwright/test"

import { createDashboard, getDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectNoAnimations,
  expectPanelsReady,
  expectSavedVersion,
  openDashboard,
  panel,
  selectPanelToArrange,
  unsavedLayout,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

const errorRateTitle = "Checkout error rate"

type GridPos = { x: number; y: number; w: number; h: number }

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E layout")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function gridPos(spec: { panels: Array<Record<string, unknown>> }, id: string) {
  const found = spec.panels.find((item) => item.id === id)
  if (!found) throw new Error(`panel ${id} is not in the spec`)
  return found.gridPos as GridPos
}

function moreButton(page: Page) {
  return page.getByRole("button", { name: /^More:/ })
}

async function box(locator: Locator) {
  const found = await locator.boundingBox()
  if (!found) throw new Error("element has no box")
  return found
}

async function dragTo(
  page: Page,
  from: { x: number; y: number },
  to: { x: number; y: number }
) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(from.x + 5, from.y + 5, { steps: 3 })
  await page.mouse.move(to.x, to.y, { steps: 20 })
  await page.mouse.up()
}

function gridItem(page: Page, panelId: string) {
  return page.locator(".react-grid-item", {
    has: page.locator(`article[data-panel-id="${panelId}"]`),
  })
}

async function saveLayout(page: Page, version: number, summary?: string) {
  await unsavedLayout(page)
    .getByRole("button", { name: `Save version ${version}` })
    .click()
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  if (summary !== undefined) {
    await dialog.getByRole("textbox", { name: "Summary" }).fill(summary)
  }
  await dialog.getByRole("button", { name: "Save", exact: true }).click()
}

test("drags and resizes a panel and saves a new version", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await selectPanelToArrange(page, errorRateTitle)

  const handle = await box(
    page.locator('article[data-panel-id="error_rate"] button.panel-drag-handle')
  )
  const logs = await box(panel(page, "Log rows by service"))
  await dragTo(
    page,
    { x: handle.x + handle.width / 2, y: handle.y + handle.height / 2 },
    { x: logs.x + 12, y: logs.y + 6 }
  )
  await expect(unsavedLayout(page)).toBeVisible()
  await expectNoAnimations(page.locator(".react-grid-item"))

  await selectPanelToArrange(page, "Shop requests per second")
  const resize = await box(
    gridItem(page, "rps").locator(".react-resizable-handle-se")
  )
  const rps = await box(panel(page, "Shop requests per second"))
  await dragTo(
    page,
    { x: resize.x + resize.width / 2, y: resize.y + resize.height / 2 },
    { x: resize.x + resize.width / 2 - rps.width / 4, y: resize.y + 10 }
  )

  await saveLayout(page, 2, "Move panels")
  await expectSavedVersion(page, 2)
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  await expect(versionButton(page)).toHaveText(/v2$/)

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(2)
  expect(gridPos(saved.spec, "rps").w).toBeLessThan(12)
  expect(gridPos(saved.spec, "error_rate")).not.toEqual({
    x: 12,
    y: 0,
    w: 6,
    h: 4,
  })
  await expect(unsavedLayout(page)).toHaveCount(0)
  await expect(moreButton(page)).toBeVisible()
})
