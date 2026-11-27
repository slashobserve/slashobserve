import { createDashboard, loadSpec, mutationHeaders, uniqueTitle } from "./api"
import {
  holdRequestPollsUntilFinished,
  reachDashboard,
  sentRequestId,
} from "./builds"
import { expectPanelsReady } from "./dashboards"
import { expect, test, type Page } from "./fixtures"

const redPrompt = "astronomy shop RED metrics by endpoint"
const followUpQuestion = "can this show requests by service?"
const suggestedChange = "add a requests by service panel"
const draftPrompt = "an unsent draft"
const neverSavesPrompt = "e2e: a slow run that never saves"
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"
const requestPath = /\/c\/([0-9a-f-]{36})$/
const dashboardPath = /\/d\/([0-9a-f-]{36})/
const revisionsApi = /\/api\/v1\/dashboards\/[0-9a-f-]{36}\/revisions$/
const openLink = { name: "Open dashboard", exact: true }

function turns(page: Page) {
  return page.getByRole("region", { name: /^Request / })
}

type ObserveBody = {
  request: {
    status: string
    dashboard_id: string | null
    steps: { phase: string }[]
  }
}

test("creates a dashboard from a prompt", async ({ page }) => {
  const release = await holdRequestPollsUntilFinished(page)

  await page.goto("/ask")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(redPrompt)
  await promptBar.press("Enter")

  const requestId = await sentRequestId(page)
  const turn = turns(page)
  await expect(turn.getByText(redPrompt, { exact: true })).toBeVisible()
  await expect(turn.getByText("Queued, position 1")).toBeVisible()
  await expect(
    turn.getByRole("list", { name: "Progress" }).getByText("Working")
  ).toHaveCount(0)
  await release(requestId)

  await reachDashboard(page)
  await expectPanelsReady(page)
  await expect(page.getByRole("alert")).toHaveCount(0)

  await page.goto(`/c/${requestId}`)
  const open = page.getByRole("link", openLink)
  await expect(open).toBeVisible()
  await expect(open).not.toHaveAttribute("target")
  await expect(page.getByText("Saved as version 1")).toBeVisible()
  await open.click()
  await expect(page).toHaveURL(dashboardPath)

  await page.goto("/ask")
  await expect(
    page
      .getByRole("region", { name: "Recent chats" })
      .getByRole("button", { name: new RegExp(redPrompt) })
      .first()
  ).toBeVisible()
})

test("records the repair step and reopens finished requests", async ({
  page,
}) => {
  const created = await page.request.post("/api/v1/observe", {
    data: { prompt: redPrompt },
    headers: mutationHeaders,
  })
  expect(created.status()).toBe(202)
  const { request } = (await created.json()) as { request: { id: string } }
  const path = `/api/v1/observe/${request.id}`

  let finished: ObserveBody["request"] | undefined
  await expect
    .poll(
      async () => {
        const response = await page.request.get(path)
        const body = (await response.json()) as ObserveBody
        finished = body.request
        return body.request.status
      },
      { timeout: 90_000, intervals: [1000] }
    )
    .toBe("succeeded")
  expect(JSON.stringify(finished?.steps)).toContain('"phase":"repair"')
  expect(finished?.dashboard_id).not.toBeNull()

  await page.goto(`/c/${request.id}`)
  await expect(page).toHaveURL(new RegExp(`/c/${request.id}$`))
  const summary = page.locator("summary").filter({ hasText: /retried 1 time/ })
  await expect(summary).toBeVisible()
  await summary.click()
  await expect(
    page.getByRole("list", { name: "Progress" }).getByText("Repairing")
  ).toBeVisible()
})

test("explains a failed request", async ({ page }) => {
  await page.goto("/ask")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(neverSavesPrompt)
  await page.getByRole("button", { name: "Send" }).click()

  await sentRequestId(page)
  const chat = turns(page)
  const alert = chat.getByRole("alert")
  await expect(alert).toContainText("without saving", { timeout: 90_000 })
  await expect(alert.getByRole("button", { name: "Try again" })).toBeVisible()
  await chat
    .locator("summary")
    .filter({ hasText: /^Ran \d+ steps?/ })
    .click()
  await expect(
    chat.getByRole("list", { name: "Progress" }).getByText("Running queries")
  ).not.toHaveCount(0)
  await expect(chat.getByText(neverSavesPrompt, { exact: true })).toBeVisible()

  const failedUrl = page.url()
  await alert.getByRole("button", { name: "Try again" }).click()
  await expect(page).not.toHaveURL(failedUrl)
  await sentRequestId(page)
  await expect(chat.getByText(neverSavesPrompt, { exact: true })).toHaveCount(2)
  await expect(alert).toHaveCount(2, { timeout: 90_000 })
  await expect(alert.last()).toContainText("without saving")
})

test("retries a failed follow-up on the same dashboard", async ({ page }) => {
  const { id: dashboardId } = await createDashboard(page.request, {
    ...loadSpec("overview"),
    title: uniqueTitle("Follow-up retry"),
  })
  const queued = await page.request.post("/api/v1/observe", {
    data: {
      prompt: unanswerablePrompt,
      dashboard_id: dashboardId,
      kind: "revise",
    },
    headers: mutationHeaders,
  })
  expect(queued.status()).toBe(202)
  const { request } = (await queued.json()) as { request: { id: string } }

  await page.goto(`/c/${request.id}`)
  const alerts = page.getByRole("alert")
  await expect(alerts).toContainText("no replay fixture", { timeout: 30_000 })
  await expect(alerts).toContainText("could not be updated")
  const open = alerts.getByRole("link", openLink)
  await expect(open).toHaveAttribute(
    "href",
    new RegExp(`^/d/${dashboardId}(/[a-z0-9-]+)?$`)
  )
  await expect(open).not.toHaveAttribute("target")
  await alerts.getByRole("button", { name: "Try again" }).click()

  await expect(alerts).toHaveCount(2, { timeout: 30_000 })
  await expect(page).toHaveURL(new RegExp(`/c/${request.id}$`))
  await expect(alerts.last()).toContainText("no replay fixture", {
    timeout: 30_000,
  })

  const revisions = await page.request.get(
    `/api/v1/dashboards/${dashboardId}/revisions`
  )
  const body = (await revisions.json()) as {
    revisions: { intent: string; prompt: string }[]
  }
  expect(body.revisions).toHaveLength(2)
  for (const revision of body.revisions) {
    expect(revision).toMatchObject({
      intent: "apply",
      prompt: unanswerablePrompt,
    })
  }
})

test("links a failed comment revision back to its dashboard", async ({
  page,
}) => {
  const { id: dashboardId } = await createDashboard(page.request, {
    ...loadSpec("overview"),
    title: uniqueTitle("Comment revision"),
  })
  const commented = await page.request.post("/api/v1/threads", {
    data: {
      target: `dashboard:${dashboardId}`,
      version: 1,
      panel_id: "rps",
      body: unanswerablePrompt,
    },
    headers: mutationHeaders,
  })
  expect(commented.status()).toBe(201)
  const { thread } = (await commented.json()) as { thread: { id: string } }
  const queued = await page.request.post(
    `/api/v1/dashboards/${dashboardId}/revisions`,
    {
      data: {
        prompt: unanswerablePrompt,
        base_version: 1,
        comment_id: thread.id,
      },
      headers: mutationHeaders,
    }
  )
  expect(queued.status()).toBe(202)
  const { revision } = (await queued.json()) as { revision: { id: string } }

  await page.goto(`/c/${revision.id}`)
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("no replay fixture", { timeout: 30_000 })
  await expect(alert.getByRole("button", { name: "Try again" })).toHaveCount(0)
  const open = alert.getByRole("link", openLink)
  await expect(open).toHaveAttribute(
    "href",
    new RegExp(`^/d/${dashboardId}(/[a-z0-9-]+)?$`)
  )
  await expect(open).not.toHaveAttribute("target")
})

test("follows up in the same chat", async ({ page }) => {
  test.setTimeout(180_000)
  const release = await holdRequestPollsUntilFinished(page)
  await page.goto("/ask")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(redPrompt)
  await promptBar.press("Enter")

  const requestId = await sentRequestId(page)
  await release(requestId)
  await reachDashboard(page)

  let openRevisions!: () => void
  const revisionsGate = new Promise<void>(
    (resolve) => (openRevisions = resolve)
  )
  await page.route(revisionsApi, async (route) => {
    await revisionsGate
    await route.continue()
  })
  await page.goto(`/c/${requestId}`)
  await expect(page).toHaveURL(requestPath)
  const conversationUrl = page.url()
  await expect(promptBar).toBeEnabled({ timeout: 90_000 })
  await promptBar.fill(draftPrompt)
  openRevisions()
  await expect(page.getByText("Saved as version 1")).toBeVisible({
    timeout: 90_000,
  })
  await expect(promptBar).toHaveValue(draftPrompt)

  const releaseFollowUp = await holdRequestPollsUntilFinished(page)
  const followUpCreated = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/v1/observe"
  )
  await promptBar.fill(followUpQuestion)
  await promptBar.press("Enter")
  const followUp = (await (await followUpCreated).json()) as {
    request: { id: string }
  }
  await expect(page).toHaveURL(conversationUrl)
  await expect(page.getByText(followUpQuestion, { exact: true })).toBeVisible()
  await releaseFollowUp(followUp.request.id)
  await expect(
    page.getByText(`Suggested change: ${suggestedChange}`)
  ).toBeVisible({ timeout: 90_000 })
  await expect(page.getByText("Saved as version 2")).toHaveCount(0)

  const revisionCreated = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/v1/observe"
  )
  await page
    .getByRole("button", { name: "Revise the dashboard", exact: true })
    .click()
  expect((await revisionCreated).status()).toBe(202)
  await expect(page.getByText("Saved as version 2")).toBeVisible({
    timeout: 90_000,
  })
  await expect(page).toHaveURL(conversationUrl)
  await expect(promptBar).toHaveAttribute(
    "placeholder",
    "Ask about Astronomy shop RED metrics"
  )
})

test("slash focuses the chat composer", async ({ page }) => {
  await page.goto("/ask")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await expect(promptBar).toBeFocused()
  await page.getByRole("heading", { level: 1 }).click()
  await expect(promptBar).not.toBeFocused()
  await page.keyboard.press("/")
  await expect(promptBar).toBeFocused()
  await expect(promptBar).toHaveValue("")
})

test.describe("touch device", () => {
  test.use({ hasTouch: true, isMobile: true })

  test("keeps Enter for new lines and submits with the button", async ({
    page,
  }) => {
    const release = await holdRequestPollsUntilFinished(page)

    await page.goto("/ask")
    const promptBar = page.getByRole("textbox", { name: "Message the agent" })
    await expect(promptBar).toHaveAttribute("enterkeyhint", "enter")
    await expect(promptBar).not.toHaveAttribute("aria-describedby")
    await expect(page.getByText("Enter sends")).toHaveCount(0)

    await promptBar.fill(redPrompt)
    await promptBar.press("Enter")
    await expect(promptBar).toHaveValue(`${redPrompt}\n`)
    await expect(page).toHaveURL(/\/ask$/)

    await page.getByRole("button", { name: "Send" }).click()
    const requestId = await sentRequestId(page)
    await expect(
      turns(page).getByText(redPrompt, { exact: true })
    ).toBeVisible()
    await release(requestId)

    await reachDashboard(page)
    await expectPanelsReady(page)
  })
})
