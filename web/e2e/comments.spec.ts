import type { APIRequestContext, Page } from "@playwright/test"

import {
  addVersion,
  createComment,
  createDashboard,
  loadSpec,
  uniqueTitle,
} from "./api"
import { expectPanelsReady, openDashboard } from "./dashboards"
import { expect, test } from "./fixtures"

const selectAll = process.platform === "darwin" ? "Meta+A" : "Control+A"
const rpsTitle = "Checkout requests per second"
const logsTitle = "Log rows by service"

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E comments")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function sheet(page: Page) {
  return page.getByRole("dialog")
}

function announcer(page: Page) {
  return sheet(page).locator('p.sr-only[role="status"]')
}

function rpsPanel(page: Page) {
  return page.locator('article[data-panel-id="rps"]')
}

function composer(page: Page, title = rpsTitle) {
  return sheet(page).getByRole("textbox", { name: `Comment on ${title}` })
}

async function openFromMenu(page: Page, title = rpsTitle) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Comment" }).click()
  await expect(sheet(page)).toBeVisible()
}

async function waitForLive(page: Page) {
  await expect(
    page.getByRole("status").filter({ hasText: /^Live$/ })
  ).toBeVisible()
}

test("comments on a panel", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  await expect(page).toHaveURL(/panel=rps&tab=comments/)
  await expect(
    sheet(page).getByText("No open comments on this panel.")
  ).toBeVisible()
  const box = composer(page)
  await box.fill("Spike at 10:00?")
  await box.press("Control+Enter")

  const item = sheet(page).getByRole("article")
  await expect(item).toHaveCount(1)
  await expect(item).toContainText("Spike at 10:00?")
  await expect(item).toContainText("admin")
  await expect(box).toHaveValue("")
  await expect(box).toBeFocused()
  await expect(
    rpsPanel(page).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
      includeHidden: true,
    })
  ).toHaveCount(1)
})

test("resolves and reopens a comment", async ({ page, request }) => {
  const { id } = await createOverview(request)
  const commentId = await createComment(request, id, 1, "rps", "Looks high")
  await openDashboard(page, id)
  await expectPanelsReady(page)
  const badge = rpsPanel(page).getByRole("button", {
    name: `1 open comment on ${rpsTitle}`,
    includeHidden: true,
  })
  await expect(badge).toHaveCount(1)
  await openFromMenu(page)

  const item = sheet(page).getByRole("article")
  await item.getByRole("button", { name: /^Resolve comment by/ }).click()
  await expect(item).toHaveCount(0)
  await expect(
    sheet(page).getByText("No open comments on this panel.")
  ).toBeVisible()
  await expect(announcer(page)).toHaveText("Comment resolved.")
  await expect(sheet(page).locator(":focus")).toContainText(
    "No open comments on this panel."
  )
  await expect(
    rpsPanel(page).getByRole("button", {
      name: /open comment/,
      includeHidden: true,
    })
  ).toHaveCount(0)

  await sheet(page)
    .getByRole("tab", { name: /^Resolved/ })
    .click()
  await expect(item).toHaveCount(1)
  await expect(item.getByText("Resolved", { exact: true })).toBeVisible()
  await item.getByRole("button", { name: /^Reopen comment by/ }).click()
  await expect(item).toHaveCount(0)
  await expect(sheet(page).getByText("No resolved comments.")).toBeVisible()

  await sheet(page).getByRole("tab", { name: /^Open/ }).click()
  await expect(item).toHaveCount(1)
  await expect(badge).toHaveCount(1)
  await expect
    .poll(async () => {
      const response = await request.get(
        `/api/v1/dashboards/${id}/comments?status=open`
      )
      const body = (await response.json()) as { comments: { id: string }[] }
      return body.comments.map((comment) => comment.id)
    })
    .toContain(commentId)
})

test("shows a teammate's comment live", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await waitForLive(page)
  await openFromMenu(page)
  await expect(
    sheet(page).getByText("No open comments on this panel.")
  ).toBeVisible()
  await composer(page).fill("Still typing")

  await createComment(request, id, 1, "rps", "From the API")

  await expect(sheet(page).getByText("From the API")).toBeVisible({
    timeout: 10_000,
  })
  await expect(composer(page)).toHaveValue("Still typing")
})

test("renders comment text as text", async ({ page, request }) => {
  const { id } = await createOverview(request)
  const body = "<img src=x onerror=alert(1)> **bold**"
  await createComment(request, id, 1, "rps", body)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  const item = sheet(page).getByRole("article")
  await expect(item).toHaveCount(1)
  await expect(item.locator("p", { hasText: "onerror" })).toHaveText(body)
  await expect(item.locator("img")).toHaveCount(0)
  await expect(item.locator("strong")).toHaveCount(0)
})

test("marks comments on removed panels", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await createComment(request, id, 1, "error_rate", "Why is this flat?")
  await addVersion(
    request,
    id,
    {
      ...spec,
      panels: spec.panels.filter((item) => item.id !== "error_rate"),
    },
    1,
    "Remove error rate"
  )
  await openDashboard(page, id)
  await expectPanelsReady(page)

  await page.getByRole("button", { name: /activity/ }).click()

  await expect(page).toHaveURL(/tab=comments/)
  await expect(page).not.toHaveURL(/panel=/)
  await expect(
    sheet(page).getByRole("heading", { level: 3, name: "error_rate (removed)" })
  ).toBeVisible()
  await expect(
    sheet(page).getByText("This panel is not in the latest version.")
  ).toBeVisible()
  await expect(
    sheet(page).getByRole("button", { name: /^Open panel/ })
  ).toHaveCount(0)
})

test("opens a panel from the dashboard-wide list", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Check the dip")
  await openDashboard(page, id)
  await expectPanelsReady(page)

  await page.getByRole("button", { name: /activity/ }).click()
  await expect(
    sheet(page).getByRole("heading", { level: 3, name: rpsTitle })
  ).toBeVisible()
  await sheet(page)
    .getByRole("button", { name: `Open panel ${rpsTitle}` })
    .click()

  await expect(page).toHaveURL(/panel=rps&tab=comments/)
  await expect(
    sheet(page).getByRole("heading", { name: rpsTitle })
  ).toBeFocused()
  await expect(composer(page)).toBeVisible()
  await expect(sheet(page).getByText("Check the dip")).toBeVisible()
})

test("keeps the limit visible", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  const box = composer(page)
  await box.fill("a".repeat(3990))
  const counter = sheet(page).getByText("10 characters left")
  await expect(counter).toBeVisible()
  await expect(counter).not.toHaveClass(/text-destructive/)
  await expect(
    sheet(page).getByRole("button", { name: "Comment" })
  ).toBeEnabled()

  await box.fill("a".repeat(4010))
  const over = sheet(page).getByText("10 characters over the limit")
  await expect(over).toBeVisible()
  await expect(over).toHaveClass(/text-destructive/)
  await expect(
    sheet(page).getByRole("button", { name: "Comment" })
  ).toBeDisabled()
})

test("opens comments from a link and returns focus", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await page.goto(`/dashboards/${id}?panel=logs&tab=comments`)

  await expect(composer(page, logsTitle)).toBeVisible()
  await sheet(page).getByRole("tab", { name: /^Open/ }).focus()
  await page.keyboard.press("Escape")

  await expect(sheet(page)).toHaveCount(0)
  await expect(
    page.getByRole("button", { name: `Panel menu: ${logsTitle}` })
  ).toBeFocused()
})

test("keeps a comment draft across tabs and asks before discarding it", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  await composer(page).fill("Half written")
  await sheet(page).getByRole("tab", { name: "Query" }).click()
  await expect(page).toHaveURL(/tab=query/)
  await expect(sheet(page).locator(".cm-content")).toContainText(
    "http_requests_total"
  )
  await sheet(page).getByRole("tab", { name: "Comments" }).click()
  await expect(composer(page)).toHaveValue("Half written")

  await page.goBack()
  const dialog = page.getByRole("alertdialog", {
    name: "Discard your comment?",
  })
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Keep editing" }).click()
  await expect(dialog).toHaveCount(0)
  await expect(sheet(page)).toBeVisible()

  await page.keyboard.press("Escape")
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  await expect(
    page.getByRole("button", { name: `Panel menu: ${rpsTitle}` })
  ).toBeFocused()
})

test("keeps comments unavailable while the layout is being edited", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Before the layout session")
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await page.getByRole("button", { name: /^More/ }).click()
  await page.getByRole("menuitem", { name: "Edit layout" }).click()
  await expect(page.getByRole("button", { name: "Save layout" })).toBeVisible()

  await expect(page.getByRole("button", { name: /activity/ })).toBeDisabled()
  await expect(
    rpsPanel(page).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
    })
  ).toBeDisabled()
  await page.getByRole("button", { name: `Panel menu: ${rpsTitle}` }).click()
  await expect(
    page.getByRole("menuitem", { name: "Comment (finish the layout first)" })
  ).toHaveAttribute("aria-disabled", "true")
})

test("asks once when a query and a comment are both unsaved", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  await composer(page).fill("Half written")
  await sheet(page).getByRole("tab", { name: "Query" }).click()
  const content = sheet(page).locator(".cm-content")
  await expect(content).toContainText("http_requests_total")
  await content.click()
  await page.keyboard.insertText("1 + ")
  await sheet(page).getByRole("button", { name: "Close" }).click()

  const dialog = page.getByRole("alertdialog", {
    name: "Discard your unsaved changes?",
  })
  await expect(dialog).toBeVisible()
  await expect(page.getByRole("alertdialog")).toHaveCount(1)
  await dialog.getByRole("button", { name: "Discard" }).click()
  await expect(sheet(page)).toHaveCount(0)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
})

test("keeps a comment draft when its panel leaves in a newer version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  const removedNotice =
    "This panel is not in the latest version (version 2), so it cannot take new comments."
  let posts = 0
  page.on("request", (item) => {
    if (item.method() === "POST" && item.url().includes("/comments")) posts++
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await waitForLive(page)
  await openFromMenu(page)
  const box = composer(page)
  await box.fill("Is this still right?")

  await addVersion(
    request,
    id,
    { ...spec, panels: spec.panels.filter((item) => item.id !== "rps") },
    1,
    "Remove rps"
  )
  await expect(page.getByText("Updated to version 2")).toBeVisible({
    timeout: 10_000,
  })
  await expect(sheet(page)).toBeVisible()
  await expect(box).toHaveValue("Is this still right?")

  await expect(sheet(page).getByRole("alert")).toHaveText(removedNotice)
  const submit = sheet(page).getByRole("button", {
    name: "Comment",
    exact: true,
  })
  await expect(submit).toBeDisabled()
  await box.press("Control+Enter")
  await expect(box).toHaveValue("Is this still right?")
  expect(posts).toBe(0)
})

test("keeps a new comment clear of the composer", async ({ page, request }) => {
  const { id } = await createOverview(request)
  for (let index = 0; index < 6; index++) {
    await createComment(
      request,
      id,
      1,
      "rps",
      `Earlier comment ${index + 1}\nwith a second line\nand a third line`
    )
  }
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await expect(sheet(page).getByRole("article")).toHaveCount(6)

  const box = composer(page)
  await box.fill("The newest comment")
  await box.press("Control+Enter")

  const newest = sheet(page).getByRole("article").last()
  await expect(newest).toContainText("The newest comment")
  await expect
    .poll(async () => {
      const item = await newest.boundingBox()
      const field = await box.boundingBox()
      if (!item || !field) return false
      return item.y + item.height <= field.y
    })
    .toBe(true)
})

function commentsList(id: string) {
  return `**/api/v1/dashboards/${id}/comments?*`
}

const internalError = JSON.stringify({ error: "boom", code: "internal" })

function dropExpectedErrors(consoleErrors: string[], status: string) {
  for (let index = consoleErrors.length - 1; index >= 0; index--) {
    if (consoleErrors[index].includes(`status of ${status}`)) {
      consoleErrors.splice(index, 1)
    }
  }
}

test("stops paging when a later page keeps failing", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "On the first page")
  let failed = 0
  await page.route(commentsList(id), async (route) => {
    const url = new URL(route.request().url())
    if (url.searchParams.has("cursor")) {
      failed++
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: internalError,
      })
      return
    }
    const response = await route.fetch()
    const body = (await response.json()) as Record<string, unknown>
    await route.fulfill({ response, json: { ...body, next_cursor: "page-2" } })
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)

  let previous = -1
  await expect
    .poll(
      () => {
        const steady = failed > 0 && failed === previous
        previous = failed
        return steady
      },
      { intervals: [1000], timeout: 15_000 }
    )
    .toBe(true)
  expect(failed).toBeLessThanOrEqual(2)
  dropExpectedErrors(consoleErrors, "500")
})

test("shows a comment once when the next page repeats it", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Appears on two pages")
  await page.route(commentsList(id), async (route) => {
    const url = new URL(route.request().url())
    const paged = url.searchParams.has("cursor")
    url.searchParams.delete("cursor")
    const response = await route.fetch({ url: url.toString() })
    const body = (await response.json()) as Record<string, unknown>
    await route.fulfill({
      response,
      json: { ...body, next_cursor: paged ? null : "page-2" },
    })
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  await expect(sheet(page).getByRole("article")).toHaveCount(1)
})

test("asks before Back drops a comment draft across a version change", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await page.goto(`/dashboards/${id}?version=1&panel=rps&tab=query`)
  await sheet(page).getByRole("button", { name: "Open latest version" }).click()
  await expect(page).not.toHaveURL(/version=/)
  await sheet(page).getByRole("tab", { name: "Comments" }).click()
  await expect(page).toHaveURL(/tab=comments/)
  await composer(page).fill("Written on the latest version")

  await page.goBack()
  const dialog = page.getByRole("alertdialog", {
    name: "Discard your comment?",
  })
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Keep editing" }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page).not.toHaveURL(/version=/)
  await expect(composer(page)).toHaveValue("Written on the latest version")

  await page.goBack()
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Discard" }).click()
  await expect(page).toHaveURL(/version=1/)
})

test("returns focus to the toolbar button when the dashboard-wide list closes", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await page.keyboard.press("Escape")
  await expect(sheet(page)).toHaveCount(0)
  await expect(
    page.getByRole("button", { name: `Panel menu: ${rpsTitle}` })
  ).toBeFocused()

  const toolbarButton = page.getByRole("button", { name: /activity/ })
  await toolbarButton.click()
  await expect(sheet(page)).toBeVisible()
  await page.keyboard.press("Escape")

  await expect(sheet(page)).toHaveCount(0)
  await expect(toolbarButton).toBeFocused()
})

test("ignores a second status change while the first is saving", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Looks high")
  let patches = 0
  let release!: () => void
  const held = new Promise<void>((resolve) => (release = resolve))
  let reached!: () => void
  const firstPatch = new Promise<void>((resolve) => (reached = resolve))
  await page.route(`**/api/v1/dashboards/${id}/comments/*`, async (route) => {
    if (route.request().method() !== "PATCH") {
      await route.fallback()
      return
    }
    if (route.request().url().endsWith("/barrier")) {
      await route.fulfill({ status: 200, json: {} })
      return
    }
    patches++
    reached()
    await held
    await route.continue()
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await sheet(page).getByRole("tab", { name: /^All/ }).click()

  const item = sheet(page)
    .getByRole("tabpanel", { name: /^All/ })
    .getByRole("article")
  await item.getByRole("button", { name: /^Resolve comment by/ }).click()
  await firstPatch
  const reopen = item.getByRole("button", { name: /^Reopen comment by/ })
  await expect(reopen).toHaveAttribute("aria-disabled", "true")
  await reopen.focus()
  await page.keyboard.press("Enter")
  await page.evaluate(
    (url) => fetch(url, { method: "PATCH" }),
    `/api/v1/dashboards/${id}/comments/barrier`
  )
  await expect(reopen).toBeFocused()
  expect(patches).toBe(1)

  release()
  await expect(reopen).not.toHaveAttribute("aria-disabled", "true")
  expect(patches).toBe(1)
})

test("announces a status change once it is saved and again when it repeats", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "First")
  await createComment(request, id, 1, "rps", "Second")
  let release!: () => void
  const held = new Promise<void>((resolve) => (release = resolve))
  let reached!: () => void
  const firstPatch = new Promise<void>((resolve) => (reached = resolve))
  await page.route(`**/api/v1/dashboards/${id}/comments/*`, async (route) => {
    if (route.request().method() !== "PATCH") {
      await route.fallback()
      return
    }
    reached()
    await held
    await route.continue()
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await sheet(page).getByRole("tab", { name: /^All/ }).click()

  const status = announcer(page)
  const text = () => status.evaluate((node) => node.textContent)
  const items = sheet(page)
    .getByRole("tabpanel", { name: /^All/ })
    .getByRole("article")
  await items
    .first()
    .getByRole("button", { name: /^Resolve comment by/ })
    .click()
  await firstPatch
  expect(await text()).toBe("")

  release()
  await expect
    .poll(async () => (await text())?.trim())
    .toBe("Comment resolved.")
  const first = await text()
  await items
    .last()
    .getByRole("button", { name: /^Resolve comment by/ })
    .click()
  await expect
    .poll(async () => {
      const next = await text()
      return next !== first && next?.trim() === "Comment resolved."
    })
    .toBe(true)
})

test("stays silent and says so in a toast when a status change fails", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Looks high")
  await page.route(`**/api/v1/dashboards/${id}/comments/*`, async (route) => {
    if (route.request().method() !== "PATCH") {
      await route.fallback()
      return
    }
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: internalError,
    })
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)

  await sheet(page)
    .getByRole("button", { name: /^Resolve comment by/ })
    .click()

  await expect(page.getByText("Could not update the comment")).toBeVisible()
  await expect(sheet(page).getByRole("article")).toHaveCount(1)
  await expect(announcer(page)).toHaveText("")
  dropExpectedErrors(consoleErrors, "500")
})

test("keeps a new comment clear of a taller composer", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createOverview(request)
  for (let index = 0; index < 6; index++) {
    await createComment(
      request,
      id,
      1,
      "rps",
      `Earlier comment ${index + 1}\nwith a second line\nand a third line`
    )
  }
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await expect(sheet(page).getByRole("article")).toHaveCount(6)

  let failNext = true
  await page.route(`**/api/v1/dashboards/${id}/comments`, async (route) => {
    if (route.request().method() === "POST" && failNext) {
      failNext = false
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: internalError,
      })
      return
    }
    await route.fallback()
  })
  const box = composer(page)
  const lines = Array.from({ length: 8 }, (_, index) => `Line ${index + 1}`)
  await box.fill(lines.join("\n"))
  await box.press("Control+Enter")
  await expect(sheet(page).getByRole("alert")).toBeVisible()
  const composerBox = box.locator(
    "xpath=ancestor::div[contains(@class, 'sticky')][1]"
  )
  const earlier = sheet(page).getByRole("article").last()
  await expect
    .poll(async () => {
      const height = (await composerBox.boundingBox())?.height ?? Infinity
      const margin = await earlier.evaluate((node) =>
        parseFloat(getComputedStyle(node).scrollMarginBottom)
      )
      return margin >= height
    })
    .toBe(true)
  await box.press("Control+Enter")

  const newest = sheet(page).getByRole("article").last()
  await expect(newest).toContainText("Line 8")
  await expect
    .poll(async () => {
      const item = await newest.boundingBox()
      const field = await sheet(page)
        .getByRole("button", { name: "Comment", exact: true })
        .boundingBox()
      if (!item || !field) return false
      return item.y + item.height <= field.y
    })
    .toBe(true)
  dropExpectedErrors(consoleErrors, "500")
})

test("offers to open a panel that the pinned version still has", async ({
  page,
  request,
}) => {
  const { id, spec } = await createOverview(request)
  await createComment(request, id, 1, "error_rate", "Why is this flat?")
  const extra = {
    ...spec.panels[0],
    id: "extra",
    title: "Extra panel",
    gridPos: { x: 0, y: 32, w: 12, h: 8 },
  }
  await addVersion(
    request,
    id,
    {
      ...spec,
      panels: [
        ...spec.panels.filter((item) => item.id !== "error_rate"),
        extra,
      ],
    },
    1,
    "Swap a panel"
  )
  await createComment(request, id, 2, "extra", "New panel note")
  await openDashboard(page, id, "?version=1")
  await expectPanelsReady(page)

  await page.getByRole("button", { name: /activity/ }).click()

  await expect(
    sheet(page).getByRole("heading", { level: 3, name: "Checkout error rate" })
  ).toBeVisible()
  await expect(
    sheet(page).getByRole("heading", { level: 3, name: "Extra panel" })
  ).toBeVisible()
  await expect(
    sheet(page).getByRole("button", { name: /^Open panel/ })
  ).toHaveCount(1)
  await sheet(page)
    .getByRole("button", { name: "Open panel Checkout error rate" })
    .click()
  await expect(page).toHaveURL(/version=1&panel=error_rate&tab=comments/)
  await expect(composer(page, "Checkout error rate")).toBeVisible()
})

test("keeps a comment draft when a saved query closes the query tab", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openFromMenu(page)
  await composer(page).fill("Not sent yet")
  await sheet(page).getByRole("tab", { name: "Query" }).click()
  const content = sheet(page).locator(".cm-content")
  await expect(content).toContainText("http_requests_total")
  const edited = 'sum(rate(http_requests_total{service="checkout"}[1m]))'
  await content.click()
  await page.keyboard.press(selectAll)
  await page.keyboard.insertText(edited)
  await expect(content).toHaveText(edited)
  await sheet(page).getByRole("button", { name: "Save as new version" }).click()

  await expect(page.getByText("Saved version 2")).toBeVisible()
  await expect(sheet(page)).toBeVisible()
  await expect(page).toHaveURL(/panel=rps&tab=comments/)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  await expect(composer(page)).toHaveValue("Not sent yet")

  await sheet(page).getByRole("tab", { name: "Query" }).click()
  await expect(content).toHaveText(edited)
  await expect(
    sheet(page).getByRole("button", { name: "Save as new version" })
  ).toBeDisabled()
})
