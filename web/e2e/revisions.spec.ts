import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { Page } from "@playwright/test"

import {
  createComment,
  createDashboard,
  getDashboard,
  uniqueTitle,
  type Spec,
} from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const repoRoot = join(import.meta.dirname, "..", "..")
const agentTimeout = 90_000

function readJson<T>(...segments: string[]): T {
  return JSON.parse(
    readFileSync(join(repoRoot, ...segments), { encoding: "utf8" })
  ) as T
}

function loadEnvelopeSpec(): Spec {
  return readJson<{ spec: Spec }>(
    "e2e",
    "fixtures",
    "api",
    "dashboard-valid.json"
  ).spec
}

function fixturePrompt(file: string): string {
  return readJson<{ prompt: string }>("e2e", "fixtures", "llm", file).prompt
}

const commentPrompt = fixturePrompt("comment-per-route.json")
const followUpPrompt = fixturePrompt("followup-p99-panel.json")
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"
const revisionsApi = /\/api\/v1\/dashboards\/[^/]+\/revisions(\?.*)?$/
const rpsTitle = "Requests per second"
const askLabel = "Ask the agent to revise this panel"

async function createRevisionDashboard(page: Page) {
  const spec = loadEnvelopeSpec()
  spec.title = uniqueTitle("E2E revisions")
  const { id } = await createDashboard(page.request, spec)
  return { id, spec }
}

function sheet(page: Page) {
  return page.getByRole("dialog")
}

function followUp(page: Page) {
  return page.getByRole("textbox", { name: "Ask for a change" })
}

function followUpForm(page: Page) {
  return page.locator("form").filter({ has: followUp(page) })
}

async function openRpsComments(page: Page) {
  await page.getByRole("button", { name: `Panel menu: ${rpsTitle}` }).click()
  await page.getByRole("menuitem", { name: "Comments" }).click()
  await expect(sheet(page)).toBeVisible()
}

function routeRevisionPosts(page: Page, status: number, body: object) {
  return page.route(revisionsApi, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    return route.fulfill({ status, json: body })
  })
}

function clearRejectedRequestLogs(consoleErrors: string[], status: number) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).not.toHaveLength(0)
  for (const message of logged) {
    consoleErrors.splice(consoleErrors.indexOf(message), 1)
  }
}

test.describe("with the replay agent", () => {
  test.setTimeout(180_000)

  test("a comment on a panel produces v2 live, history shows the diff, and v1 restores", async ({
    page,
  }) => {
    const { id, spec } = await createRevisionDashboard(page)
    await openDashboard(page, id)
    await expectPanelsReady(page)
    await page.evaluate(() => {
      ;(window as unknown as { sameDocument: boolean }).sameDocument = true
    })

    await openRpsComments(page)
    const entries = await page.evaluate(() => history.length)
    const box = sheet(page).getByRole("textbox", {
      name: `Comment on ${rpsTitle}`,
    })
    await expect(
      sheet(page).getByRole("checkbox", { name: askLabel })
    ).toBeChecked()
    await box.fill(commentPrompt)
    await box.press("Control+Enter")

    const item = sheet(page).getByRole("article")
    await expect(item).toContainText(commentPrompt)
    await expect(item.getByRole("status")).toContainText(
      /Queued for the agent|Agent is working|Created version 2/
    )
    await expect(item.getByRole("status")).toContainText("Created version 2", {
      timeout: agentTimeout,
    })
    await expect(page.getByText("v2", { exact: true })).toBeVisible()
    expect(
      await page.evaluate(
        () =>
          (window as unknown as { sameDocument?: boolean }).sameDocument ===
          true
      )
    ).toBe(true)
    expect(await page.evaluate(() => history.length)).toBe(entries)

    await page.keyboard.press("Escape")
    await expect(sheet(page)).toHaveCount(0)
    await page.getByRole("link", { name: "History" }).click()
    const rows = page.getByRole("table", { name: "Versions" }).getByRole("row")
    await expect(rows).toHaveCount(3)
    await expect(rows.nth(1)).toContainText("v2")
    await expect(rows.nth(1)).toContainText("Show requests per second by route")
    await expect(
      page.getByRole("heading", { level: 2, name: "Changes from v1 to v2" })
    ).toBeVisible()
    await expect(
      page.getByText("Changed", { exact: true }).first()
    ).toBeVisible()

    await page.getByRole("button", { name: "Restore version 1" }).click()
    const dialog = page.getByRole("alertdialog")
    await dialog.getByRole("button", { name: "Restore", exact: true }).click()
    await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
    await expect(
      page.getByRole("main").getByText("v3", { exact: true })
    ).toBeVisible()

    const restored = await getDashboard(page.request, id)
    expect(restored.latestVersion).toBe(3)
    const shape = (panels: Spec["panels"]) =>
      panels.map((item) => ({ id: item.id, query: item.query }))
    expect(shape(restored.spec.panels)).toEqual(shape(spec.panels))
  })

  test("a follow-up prompt produces a new version", async ({ page }) => {
    const { id } = await createRevisionDashboard(page)
    await openDashboard(page, id)
    await expectPanelsReady(page)

    const field = followUp(page)
    await field.fill(followUpPrompt)
    await field.press("Enter")

    const form = followUpForm(page)
    await expect(form.getByRole("status")).toContainText(
      /Queued for the agent|Agent is working|Created version 2/
    )
    await expect(form.getByText("Created version 2")).toBeVisible({
      timeout: agentTimeout,
    })
    await expect(field).toHaveValue("")
    await expect(field).toBeFocused()
    await expect(panel(page, "p99 latency")).toBeVisible()
    await expectPanelsReady(page)
    await expect(page.getByText("v2", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: /^Revisions/ }).click()
    const list = page.getByRole("dialog", { name: "Revisions" })
    await expect(list).toContainText("Prompt")
    await expect(list).toContainText(followUpPrompt)
    await expect(list.getByText("Created version 2")).toBeVisible()
  })

  test("shows queued until the list arrives, then the version", async ({
    page,
  }) => {
    const { id } = await createRevisionDashboard(page)
    await openDashboard(page, id)
    await expectPanelsReady(page)

    let release!: () => void
    const gate = new Promise<void>((resolve) => (release = resolve))
    await page.route(revisionsApi, async (route) => {
      if (route.request().method() !== "GET") return route.fallback()
      await gate
      return route.fallback()
    })

    const field = followUp(page)
    await field.fill(followUpPrompt)
    await field.press("Enter")

    const form = followUpForm(page)
    await expect(form.getByText("Queued for the agent")).toBeVisible()
    release()
    await expect(form.getByText("Created version 2")).toBeVisible({
      timeout: agentTimeout,
    })
  })

  test("shows the working state with its attempt, then the version", async ({
    page,
  }) => {
    const { id } = await createRevisionDashboard(page)
    await openDashboard(page, id)
    await expectPanelsReady(page)

    await page.route(revisionsApi, async (route) => {
      if (route.request().method() !== "GET") return route.fallback()
      const response = await route.fetch()
      const body = (await response.json()) as {
        revisions: Record<string, unknown>[]
      }
      body.revisions = body.revisions.map((revision) => ({
        ...revision,
        status: "running",
        attempts: 2,
        result_version: null,
        finished_at: null,
      }))
      return route.fulfill({ response, json: body })
    })

    const field = followUp(page)
    await field.fill(followUpPrompt)
    await field.press("Enter")

    const form = followUpForm(page)
    await expect(form.getByText("Agent is working (attempt 2)")).toBeVisible()
    await page.unroute(revisionsApi)
    await expect(form.getByText("Created version 2")).toBeVisible({
      timeout: agentTimeout,
    })
  })

  test("shows a failure with its reason and links to the steps", async ({
    page,
  }) => {
    const { id } = await createRevisionDashboard(page)
    await openDashboard(page, id)
    await expectPanelsReady(page)

    const field = followUp(page)
    await field.fill(unanswerablePrompt)
    await field.press("Enter")

    const form = followUpForm(page)
    await expect(form.getByText("Revision failed")).toBeVisible({
      timeout: agentTimeout,
    })
    await expect(form.getByRole("status")).toContainText("no replay fixture")
    await form.getByRole("link", { name: "View progress" }).click()

    await expect(page).toHaveURL(/\/observe\/[0-9a-f-]{36}$/)
    const alert = page.getByRole("alert")
    await expect(alert).toContainText("could not be updated")
    await expect(alert).toContainText("no replay fixture")
    await alert.getByRole("link", { name: "Back to the dashboard" }).click()
    await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
    await expect(followUpForm(page).getByText("Revision failed")).toBeVisible()
    await followUpForm(page).getByRole("button", { name: "Dismiss" }).click()
    await expect(followUp(page)).toBeFocused()
    await expect(followUpForm(page).getByText("Revision failed")).toHaveCount(0)
  })
})

test("rate-limits per dashboard from the follow-up bar", async ({
  page,
  consoleErrors,
}) => {
  const { id } = await createRevisionDashboard(page)
  await routeRevisionPosts(page, 429, {
    error: "too many revisions",
    code: "too_many_revisions",
  })
  await openDashboard(page, id)

  const field = followUp(page)
  await field.fill(followUpPrompt)
  await field.press("Enter")

  await expect(page.getByRole("alert")).toHaveText(
    "This dashboard already has 5 revisions waiting."
  )
  await expect(field).toHaveValue(followUpPrompt)
  await expect(field).toBeFocused()
  clearRejectedRequestLogs(consoleErrors, 429)
})

test("rate-limits per dashboard from a comment", async ({
  page,
  consoleErrors,
}) => {
  const { id } = await createRevisionDashboard(page)
  await page.route(/\/api\/v1\/dashboards\/[^/]+\/comments$/, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    return route.fulfill({
      status: 429,
      json: { error: "too many revisions", code: "too_many_revisions" },
    })
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openRpsComments(page)

  const box = sheet(page).getByRole("textbox", {
    name: `Comment on ${rpsTitle}`,
  })
  await box.fill("Please split this by route")
  await box.press("Control+Enter")

  await expect(sheet(page).getByRole("alert")).toHaveText(
    "This dashboard already has 5 revisions waiting. Your comment was not saved; try again when one finishes."
  )
  await expect(box).toHaveValue("Please split this by route")
  clearRejectedRequestLogs(consoleErrors, 429)
})

test("posts a comment without a revision request when the box is unchecked", async ({
  page,
}) => {
  const { id } = await createRevisionDashboard(page)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await openRpsComments(page)

  await sheet(page).getByRole("checkbox", { name: askLabel }).click()
  await expect(
    sheet(page).getByRole("checkbox", { name: askLabel })
  ).not.toBeChecked()
  const box = sheet(page).getByRole("textbox", {
    name: `Comment on ${rpsTitle}`,
  })
  await box.fill("Just a note for the team")
  await box.press("Control+Enter")

  const item = sheet(page).getByRole("article")
  await expect(item).toContainText("Just a note for the team")
  await expect(item.getByRole("status")).toHaveCount(0)
  const revisions = await page.request.get(`/api/v1/dashboards/${id}/revisions`)
  expect(await revisions.json()).toMatchObject({ revisions: [] })
})

test("hides revision controls once the server reports no agent", async ({
  page,
  consoleErrors,
}) => {
  const { id } = await createRevisionDashboard(page)
  await routeRevisionPosts(page, 503, {
    error: "agent unavailable",
    code: "agent_unavailable",
  })
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const field = followUp(page)
  await field.fill(followUpPrompt)
  await field.press("Enter")

  const alert = page.getByRole("alert")
  await expect(alert).toContainText("the agent is off")
  await expect(alert).toBeFocused()
  await expect(followUp(page)).toHaveCount(0)
  clearRejectedRequestLogs(consoleErrors, 503)

  await openRpsComments(page)
  await expect(
    sheet(page).getByRole("textbox", { name: `Comment on ${rpsTitle}` })
  ).toBeVisible()
  await expect(sheet(page).getByRole("checkbox")).toHaveCount(0)
})

test("shows no revision controls when no agent is configured", async ({
  page,
}) => {
  const { id } = await createRevisionDashboard(page)
  await page.route("**/api/v1/agent", (route) =>
    route.fulfill({
      json: { available: false, provider: null, model: null },
    })
  )
  await openDashboard(page, id)
  await expectPanelsReady(page)

  await expect(followUp(page)).toHaveCount(0)
  await expect(page.getByRole("button", { name: /^Revisions/ })).toHaveCount(0)
  await openRpsComments(page)
  await expect(sheet(page).getByRole("checkbox")).toHaveCount(0)
})

test("keeps a follow-up draft through History and back", async ({ page }) => {
  const { id } = await createRevisionDashboard(page)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await followUp(page).fill("Add a panel for error rate")

  await page.getByRole("link", { name: "History" }).click()
  await expect(
    page.getByRole("heading", { level: 1, name: "Version history" })
  ).toBeVisible()
  await page.goBack()

  await expect(followUp(page)).toHaveValue("Add a panel for error rate")
})

test("does not offer the follow-up bar on a pinned version", async ({
  page,
}) => {
  const { id } = await createRevisionDashboard(page)
  await openDashboard(page, id, "?version=1")
  await expectPanelsReady(page)
  await expect(followUp(page)).toHaveCount(0)
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("keeps the follow-up bar and toolbar inside the viewport", async ({
    page,
  }) => {
    const { id } = await createRevisionDashboard(page)
    await createComment(page.request, id, 1, "rps", "Open comment")
    await openDashboard(page, id)
    await expectPanelsReady(page)
    await expect(
      page.getByRole("button", { name: "Comments, 1 open" })
    ).toBeVisible()

    const field = await followUp(page).boundingBox()
    const send = await page
      .getByRole("button", { name: "Ask agent" })
      .boundingBox()
    expect(field?.x).toBeCloseTo(16, 0)
    expect((field?.x ?? 0) + (field?.width ?? 0)).toBeCloseTo(374, 0)
    expect(send?.x).toBeCloseTo(16, 0)
    expect((send?.x ?? 0) + (send?.width ?? 0)).toBeCloseTo(374, 0)

    const revisions = await page
      .getByRole("button", { name: /^Revisions/ })
      .boundingBox()
    expect((revisions?.x ?? 0) + (revisions?.width ?? 0)).toBeCloseTo(374, 0)

    const history = await page
      .getByRole("link", { name: "History" })
      .boundingBox()
    expect((history?.x ?? 0) + (history?.width ?? 0)).toBeCloseTo(374, 0)
    const overflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth
    )
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
