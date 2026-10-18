import type { APIRequestContext, Locator, Page } from "@playwright/test"

import {
  addVersion,
  createDashboard,
  getDashboard,
  loadSpec,
  uniqueTitle,
} from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
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

function moveHandle(page: Page, title: string) {
  return page.getByRole("button", { name: `Move or resize ${title}` })
}

function liveRegion(page: Page) {
  return page.locator('div[aria-live="polite"].sr-only')
}

async function startEditing(page: Page) {
  await page.getByRole("button", { name: "Edit layout" }).click()
  await expect(page.getByRole("button", { name: "Save layout" })).toBeVisible()
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

async function saveLayout(page: Page, summary?: string) {
  await page.getByRole("button", { name: "Save layout" }).click()
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  if (summary !== undefined) {
    await dialog.getByRole("textbox", { name: "Summary" }).fill(summary)
  }
  await dialog.getByRole("button", { name: "Save", exact: true }).click()
}

function expectConflictLogged(consoleErrors: string[]) {
  const conflicts = consoleErrors.filter((message) =>
    message.includes("status of 409")
  )
  expect(conflicts).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(conflicts[0]), 1)
}

test("drags and resizes a panel and saves a new version", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startEditing(page)

  const handle = await box(
    page.locator('article[data-panel-id="error_rate"] button.panel-drag-handle')
  )
  const logs = await box(panel(page, "Log rows by service"))
  await dragTo(
    page,
    { x: handle.x + handle.width / 2, y: handle.y + handle.height / 2 },
    { x: logs.x + 12, y: logs.y + 6 }
  )
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  const resize = await box(
    gridItem(page, "rps").locator(".react-resizable-handle")
  )
  const rps = await box(panel(page, "Checkout requests per second"))
  await dragTo(
    page,
    { x: resize.x + resize.width / 2, y: resize.y + resize.height / 2 },
    { x: resize.x + resize.width / 2 - rps.width / 4, y: resize.y + 10 }
  )

  await saveLayout(page, "Move panels")
  await expect(page.getByText("Saved version 2")).toBeVisible()
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  await expect(page.getByText("v2", { exact: true })).toBeVisible()

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(2)
  expect(gridPos(saved.spec, "rps").w).toBeLessThan(12)
  expect(gridPos(saved.spec, "error_rate")).not.toEqual({
    x: 12,
    y: 0,
    w: 6,
    h: 4,
  })
  await expect(page.getByRole("button", { name: "Edit layout" })).toBeVisible()
})

test("moves and resizes with the keyboard", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("Shift+ArrowLeft")
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("Shift+ArrowDown")
  await expect(moveHandle(page, errorRateTitle)).toBeFocused()
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 14, row 1, 5 by 5`
  )

  await saveLayout(page)
  await expect(page.getByText("Saved version 2")).toBeVisible()
  const saved = await getDashboard(request, id)
  expect(gridPos(saved.spec, "error_rate")).toEqual({
    x: 13,
    y: 0,
    w: 5,
    h: 5,
  })

  await startEditing(page)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Save layout" })).toBeDisabled()
})

test("moves a panel below and back above another with the keyboard", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startEditing(page)

  const handle = moveHandle(page, errorRateTitle)
  await handle.focus()
  await page.keyboard.press("ArrowDown")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 13, row 17, 6 by 4`
  )
  await expect(handle).toBeFocused()

  await page.keyboard.press("ArrowUp")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 13, row 1, 6 by 4`
  )
  await expect(handle).toBeFocused()
  await expect(page.getByText("Unsaved changes")).toHaveCount(0)
})

test("reapplies the layout after a conflict", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("ArrowRight")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 15, row 1, 6 by 4`
  )

  const title = uniqueTitle("E2E layout renamed")
  await addVersion(request, id, { ...spec, title }, 1, "Rename")
  await expect(
    page.getByText("Version 2 was saved while you were editing. Saving will")
  ).toBeVisible()
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)

  await saveLayout(page)
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  await expect(
    dialog.getByText("Version 2 was saved while you were editing.")
  ).toBeVisible()
  expectConflictLogged(consoleErrors)

  await dialog
    .getByRole("button", { name: "Reapply my layout to version 2" })
    .click()
  await expect(page.getByText("Saved version 3")).toBeVisible()

  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(3)
  expect(saved.title).toBe(title)
  expect(gridPos(saved.spec, "error_rate")).toEqual({
    x: 14,
    y: 0,
    w: 6,
    h: 4,
  })
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title)
})

test("asks before discarding unsaved changes", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  const before = await box(panel(page, errorRateTitle))
  await startEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  await page.getByRole("link", { name: "SlashObserve" }).click()
  const dialog = page.getByRole("alertdialog", {
    name: "Discard unsaved layout changes?",
  })
  await expect(dialog).toBeVisible()
  await expect(
    dialog.getByRole("button", { name: "Keep editing" })
  ).toBeFocused()
  await dialog.getByRole("button", { name: "Keep editing" }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  await page.getByRole("button", { name: "Cancel" }).click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Discard" }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Edit layout" })).toBeVisible()
  await expect(page.getByRole("button", { name: "Save layout" })).toHaveCount(0)
  await expect
    .poll(async () => (await box(panel(page, errorRateTitle))).x)
    .toBeCloseTo(before.x, 0)
  const saved = await getDashboard(request, id)
  expect(saved.latestVersion).toBe(1)
  expect(gridPos(saved.spec, "error_rate")).toMatchObject({ x: 12, y: 0 })
})

test("is unavailable on a pinned old version", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await openDashboard(page, id, "?version=1")

  const edit = page.getByRole("button", { name: "Edit layout" })
  await expect(edit).toBeDisabled()
  await page
    .locator('span[data-slot="tooltip-trigger"]')
    .filter({ has: edit })
    .hover()
  await expect(
    page.getByText("Open the latest version to edit the layout")
  ).toBeVisible()
})

test("does not block time range changes while editing", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  await page.getByRole("button", { name: /^Time range/ }).click()
  await page.getByRole("button", { name: "Last 1 hour", exact: true }).click()
  await expect(page).toHaveURL(/from=now-1h&to=now/)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Save layout" })).toBeEnabled()
  await expect(page.getByText("Unsaved changes")).toBeVisible()
})
