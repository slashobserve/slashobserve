import type { APIRequestContext, Locator, Page } from "@playwright/test"

import {
  addVersion,
  createDashboard,
  getDashboard,
  loadSpec,
  uniqueTitle,
  type Spec,
} from "./api"
import {
  expectNoAnimations,
  expectPanelsReady,
  openDashboard,
  panel,
  startLayoutEditing,
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

function moveHandle(page: Page, title: string) {
  return page.getByRole("button", { name: `Move or resize ${title}` })
}

async function openLatest(page: Page) {
  await versionButton(page).click()
  await page.getByRole("menuitemradio", { name: /Latest/ }).click()
}

function liveRegion(page: Page) {
  return page.locator('div[aria-live="polite"].sr-only')
}

function editLayoutButton(page: Page) {
  return page.getByRole("button", { name: /^More/ })
}

async function recordAnnouncements(page: Page) {
  await page.addInitScript(() => {
    const seen: string[] = []
    Object.assign(window, { announcements: seen })
    new MutationObserver(() => {
      const region = document.querySelector('div[aria-live="polite"].sr-only')
      const text = region?.textContent?.replace(/\s+/g, " ").trim() ?? ""
      if (text !== "" && seen[seen.length - 1] !== text) seen.push(text)
    }).observe(document, {
      subtree: true,
      childList: true,
      characterData: true,
    })
  })
}

function announcements(page: Page) {
  return page.evaluate(
    () => (window as unknown as { announcements: string[] }).announcements
  )
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
  await startLayoutEditing(page)

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
  await expect(editLayoutButton(page)).toBeVisible()
})

test("moves and resizes with the keyboard", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

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
  await expect(editLayoutButton(page)).toBeFocused()
  const saved = await getDashboard(request, id)
  expect(gridPos(saved.spec, "error_rate")).toEqual({
    x: 13,
    y: 0,
    w: 5,
    h: 5,
  })

  await startLayoutEditing(page)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Save layout" })).toBeDisabled()
})

test("moves a panel below and back above another with the keyboard", async ({
  page,
  request,
}) => {
  await recordAnnouncements(page)
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

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
  expect(await announcements(page)).toEqual([
    `${errorRateTitle}: column 13, row 17, 6 by 4`,
    `${errorRateTitle}: column 13, row 1, 6 by 4`,
  ])
})

test("announces when a key press changes nothing and clears after a mouse drag", async ({
  page,
  request,
}) => {
  await recordAnnouncements(page)
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  const rpsTitle = "Checkout requests per second"
  await moveHandle(page, rpsTitle).focus()
  await page.keyboard.press("ArrowLeft")
  await expect(liveRegion(page)).toHaveText(`${rpsTitle}: can't move further`)
  await page.keyboard.press("ArrowLeft")
  await expect(page.getByText("Unsaved changes")).toHaveCount(0)

  await page.keyboard.press("ArrowRight")
  await expect(liveRegion(page)).toHaveText(
    `${rpsTitle}: column 2, row 1, 12 by 8`
  )

  await expectNoAnimations(gridItem(page, "rps"))
  const resize = await box(
    gridItem(page, "rps").locator(".react-resizable-handle")
  )
  await dragTo(
    page,
    { x: resize.x + resize.width / 2, y: resize.y + resize.height / 2 },
    { x: resize.x + resize.width / 2 - 80, y: resize.y + resize.height / 2 }
  )
  await expect(liveRegion(page)).toHaveText("")
})

test("pushes a neighbour down when a keyboard move overlaps it", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  const handle = moveHandle(page, errorRateTitle)
  await handle.focus()
  await page.keyboard.press("ArrowRight")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 14, row 1, 6 by 4`
  )
  await expect
    .poll(async () => (await box(panel(page, "Requests by pod"))).y)
    .toBeGreaterThan((await box(panel(page, errorRateTitle))).y)

  await page.keyboard.press("ArrowLeft")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 13, row 1, 6 by 4`
  )
  await expect(handle).toBeFocused()
})

test("reapplies the layout after a conflict", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

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
  const reapply = dialog.getByRole("button", {
    name: "Reapply my layout to version 2",
  })
  await expect(reapply).toBeFocused()
  await expect(reapply).toHaveAccessibleDescription(
    "Version 2 was saved while you were editing."
  )
  await expect(dialog.getByRole("alert")).toHaveCount(0)

  await reapply.click()
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
  await startLayoutEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  await page.getByRole("link", { name: "/observe home" }).click()
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
  await expect(editLayoutButton(page)).toBeFocused()
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

  await editLayoutButton(page).click()
  await expect(
    page.getByRole("menuitem", {
      name: "Edit layout (open the latest version first)",
    })
  ).toHaveAttribute("aria-disabled", "true")
})

test("locks the version menu while editing", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  await versionButton(page).click()
  const menu = page.getByRole("menu", { name: "Versions" })
  await expect(
    menu.getByText("Save or cancel your layout changes to switch versions.")
  ).toBeVisible()
  const rows = menu.getByRole("menuitemradio")
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toHaveAttribute("aria-disabled", "true")
  await expect(rows.nth(1)).toHaveAttribute("aria-disabled", "true")

  const url = page.url()
  await rows.nth(1).click({ force: true })
  await expect(page).toHaveURL(url)
  await expect(versionButton(page)).toHaveText(/^(Live|Connecting) · v2$/)
  await expect(page.getByRole("button", { name: "Save layout" })).toBeVisible()
})

test("does not block time range changes while editing", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

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

test("returns focus to Edit layout after cancelling a clean session", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  await page.getByRole("button", { name: "Cancel" }).click()
  await expect(editLayoutButton(page)).toBeFocused()
  await expect(page.getByRole("button", { name: "Save layout" })).toHaveCount(0)
})

test("keeps the other version's layout changes when reapplying", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("Shift+ArrowDown")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 13, row 1, 6 by 5`
  )

  const other = structuredClone(spec)
  const logs = other.panels.find((item) => item.id === "logs")
  if (!logs) throw new Error("logs panel is missing")
  logs.gridPos = { x: 0, y: 8, w: 24, h: 12 }
  const rps = other.panels.find((item) => item.id === "rps")
  if (!rps) throw new Error("rps panel is missing")
  rps.gridPos = { x: 0, y: 0, w: 12, h: 6 }
  await addVersion(request, id, other, 1, "Resize panels")

  await saveLayout(page)
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  await dialog
    .getByRole("button", { name: "Reapply my layout to version 2" })
    .click()
  await expect(page.getByText("Saved version 3")).toBeVisible()
  expectConflictLogged(consoleErrors)

  const saved = await getDashboard(request, id)
  expect(gridPos(saved.spec, "error_rate")).toEqual({
    x: 12,
    y: 0,
    w: 6,
    h: 5,
  })
  expect(gridPos(saved.spec, "rps")).toEqual({ x: 0, y: 0, w: 12, h: 6 })
  expect(gridPos(saved.spec, "logs").h).toBe(12)
})

test("shows no edit controls on a pinned version and keeps the edits", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await openDashboard(page, id, "?version=1")
  await openLatest(page)
  await expect(page).not.toHaveURL(/version=1/)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByText("Unsaved changes")).toBeVisible()

  await page.goBack()
  await expect(page).toHaveURL(/version=1/)
  await expect(versionButton(page)).toHaveText("v1 of 2")
  await editLayoutButton(page).click()
  const edit = page.getByRole("menuitem", {
    name: "Edit layout (open the latest version first)",
  })
  await expect(edit).toBeVisible()
  await expect(page.getByRole("menu")).toBeFocused()
  await expect(edit).toHaveAttribute("aria-disabled", "true")
  await page.keyboard.press("Escape")
  await expect(edit).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Save layout" })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Cancel" })).toHaveCount(0)
  await expect(page.getByText("Unsaved changes")).toHaveCount(0)

  await openLatest(page)
  await expect(page).not.toHaveURL(/version=1/)
  await expect(page.getByRole("button", { name: "Save layout" })).toBeEnabled()
  await expect(page.getByText("Unsaved changes")).toBeVisible()
  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")
  await expect(liveRegion(page)).toHaveText(
    `${errorRateTitle}: column 15, row 1, 6 by 4`
  )
})

test("offers Try again when the server cannot check the layout right now", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)
  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")

  await page.route(/\/versions$/, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    return route.fulfill({
      status: 422,
      contentType: "application/json",
      body: JSON.stringify({
        error: "spec is invalid",
        code: "invalid_spec",
        issues: [
          {
            path: "/panels/0/queries/0/query",
            code: "datasource_unavailable",
            message: "prometheus returned 502",
          },
          {
            path: "/panels/2/queries/0/query",
            code: "query_timeout",
            message: "query timed out",
          },
        ],
      }),
    })
  })
  await saveLayout(page)
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  await expect(
    dialog.getByText("The panel queries could not be checked right now.")
  ).toHaveCount(1)
  await expect(dialog).not.toContainText("The server rejected this layout")
  await expect(dialog.getByRole("button", { name: "Try again" })).toBeVisible()
  const failed = consoleErrors.filter((message) =>
    message.includes("status of 422")
  )
  expect(failed).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(failed[0]), 1)

  await page.unroute(/\/versions$/)
  await dialog.getByRole("button", { name: "Try again" }).click()
  await expect(page.getByText("Saved version 2")).toBeVisible()
})

test("lists each rejected layout issue once", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)
  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")

  await page.route(/\/versions$/, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    const issue = {
      path: "/panels/1/gridPos",
      code: "grid_overlap",
      message: 'panel "error_rate" overlaps panel "rps"',
    }
    return route.fulfill({
      status: 422,
      contentType: "application/json",
      body: JSON.stringify({
        error: "spec is invalid",
        code: "invalid_spec",
        issues: [issue, issue],
      }),
    })
  })
  await saveLayout(page)
  const dialog = page.getByRole("dialog", { name: "Save layout" })
  await expect(
    dialog.getByText("The server rejected this layout:")
  ).toBeVisible()
  await expect(dialog.getByRole("listitem")).toHaveText([
    'panel "error_rate" overlaps panel "rps"',
  ])
  const failed = consoleErrors.filter((message) =>
    message.includes("status of 422")
  )
  expect(failed).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(failed[0]), 1)
})

test("does not flash the newer version notice on its own save", async ({
  page,
  request,
}) => {
  await page.addInitScript(() => {
    Object.assign(window, { noticeSeen: false })
    new MutationObserver(() => {
      if (
        document.documentElement.textContent?.includes(
          "was saved while you were editing"
        )
      ) {
        Object.assign(window, { noticeSeen: true })
      }
    }).observe(document, {
      subtree: true,
      childList: true,
      characterData: true,
    })
  })
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)
  await moveHandle(page, errorRateTitle).focus()
  await page.keyboard.press("ArrowRight")

  await saveLayout(page)
  await expect(page.getByText("Saved version 2")).toBeVisible()
  await expect(editLayoutButton(page)).toBeFocused()
  expect(
    await page.evaluate(
      () => (window as unknown as { noticeSeen: boolean }).noticeSeen
    )
  ).toBe(false)
})

test("shows a single resize grip on hover", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await startLayoutEditing(page)

  const item = gridItem(page, "rps")
  await item.hover()
  const handle = item.locator(".react-resizable-handle")
  await expect(handle).toHaveCSS("opacity", "1")
  const after = await handle.evaluate(
    (element) => getComputedStyle(element, "::after").display
  )
  expect(after).toBe("none")
  await expect(item.locator("svg.size-2\\.5")).toHaveCount(1)
})

type PanelBox = GridPos & { id: string }

function visibleBoxes(spec: Spec): PanelBox[] {
  const box = (item: Record<string, unknown>) => ({
    id: String(item.id),
    ...(item.gridPos as GridPos),
  })
  return spec.panels.flatMap((item) => {
    const children =
      item.type === "row" && item.collapsed !== true
        ? ((item.panels ?? []) as Array<Record<string, unknown>>)
        : []
    return [box(item), ...children.map(box)]
  })
}

function overlapping(boxes: PanelBox[]): string[] {
  const found: string[] = []
  boxes.forEach((a, index) => {
    for (const b of boxes.slice(index + 1)) {
      if (
        a.x < b.x + b.w &&
        b.x < a.x + a.w &&
        a.y < b.y + b.h &&
        b.y < a.y + a.h
      ) {
        found.push(`${a.id} overlaps ${b.id}`)
      }
    }
  })
  return found
}

test("saves a layout change on a dashboard with an expanded row", async ({
  page,
  request,
}) => {
  const spec = loadSpec("evolved")
  spec.title = uniqueTitle("E2E layout rows")
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await expect(
    page.getByRole("heading", { name: "Prometheus", level: 2 })
  ).toBeVisible()
  await startLayoutEditing(page)

  await moveHandle(page, "Log rows").focus()
  await page.keyboard.press("ArrowRight")
  await expect(moveHandle(page, "Log rows")).toBeFocused()

  await saveLayout(page)
  await expect(page.getByText("Saved version 2")).toBeVisible()
  const saved = await getDashboard(request, id)
  expect(gridPos(saved.spec, "p_logs")).toMatchObject({ x: 1, y: 0 })
  expect(gridPos(saved.spec, "p_note")).toMatchObject({ x: 12, y: 4 })
  expect(gridPos(saved.spec, "r_prom")).toEqual({ x: 0, y: 8, w: 24, h: 1 })
  expect(overlapping(visibleBoxes(saved.spec))).toEqual([])
})
