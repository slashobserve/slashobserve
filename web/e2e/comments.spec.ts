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

function rpsPanel(page: Page) {
  return page.locator('article[data-panel-id="rps"]')
}

function composer(page: Page, title = rpsTitle) {
  return sheet(page).getByRole("textbox", { name: `Comment on ${title}` })
}

async function openFromMenu(page: Page, title = rpsTitle) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: "Comments" }).click()
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
  await expect(sheet(page).getByRole("status")).toHaveText("Comment resolved.")
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

  await page.getByRole("button", { name: /^Comments/ }).click()

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

  await page.getByRole("button", { name: /^Comments/ }).click()
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
  await page.getByRole("button", { name: "Edit layout" }).click()
  await expect(page.getByRole("button", { name: "Save layout" })).toBeVisible()

  await expect(page.getByRole("button", { name: /^Comments/ })).toBeDisabled()
  await expect(
    rpsPanel(page).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
    })
  ).toBeDisabled()
  await page.getByRole("button", { name: `Panel menu: ${rpsTitle}` }).click()
  await expect(
    page.getByRole("menuitem", { name: "Comments (finish layout first)" })
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
  consoleErrors,
}) => {
  const { id, spec } = await createOverview(request)
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

  await box.press("Control+Enter")
  await expect(sheet(page).getByRole("alert")).toHaveText(
    "This panel is not in version 2. Open the latest version to comment."
  )
  await expect(box).toHaveValue("Is this still right?")
  const logged = consoleErrors.filter((message) =>
    message.includes("status of 422")
  )
  expect(logged).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(logged[0]), 1)
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
