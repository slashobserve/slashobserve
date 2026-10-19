import { createDashboard, loadSpec, mutationHeaders, uniqueTitle } from "./api"
import { expectPanelsReady } from "./dashboards"
import { expect, test } from "./fixtures"

const redPrompt = "demo shop RED metrics by endpoint"
const followUpPrompt = "add a requests by pod panel"
const neverSavesPrompt = "e2e: a slow run that never saves"
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"
const requestPath = /\/observe\/([0-9a-f-]{36})$/
const dashboardPath = /\/dashboards\/([0-9a-f-]{36})/
const requestApi = /\/api\/v1\/observe\/[0-9a-f-]{36}$/
const openLink = { name: "Open dashboard (opens in a new tab)" }

type ObserveBody = {
  request: {
    status: string
    dashboard_id: string | null
    steps: { phase: string }[]
  }
}

test("creates a dashboard from a prompt", async ({ page, context }) => {
  let openGate!: () => void
  const gate = new Promise<void>((resolve) => (openGate = resolve))
  await page.route(requestApi, async (route) => {
    await gate
    await route.continue()
  })

  await page.goto("/")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(redPrompt)
  await promptBar.press("Enter")

  await expect(page).toHaveURL(requestPath)
  const conversationUrl = page.url()
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(redPrompt)
  await expect(
    page.getByRole("list", { name: "Progress" }).getByText("Working")
  ).toBeVisible()
  openGate()

  const open = page.getByRole("link", openLink)
  await expect(open).toBeVisible({ timeout: 90_000 })
  await expect(open).toHaveAttribute("target", "_blank")
  await expect(page.getByText("Saved as version 1")).toBeVisible()
  await expect(page).toHaveURL(conversationUrl)

  const popupPromise = context.waitForEvent("page")
  await open.click()
  const popup = await popupPromise
  await expect(popup).toHaveURL(dashboardPath)
  await expectPanelsReady(popup)
  await expect(popup.getByRole("alert")).toHaveCount(0)
  await popup.close()

  await page.goto("/")
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

  await page.goto(`/observe/${request.id}`)
  await expect(page).toHaveURL(new RegExp(`/observe/${request.id}$`))
  const summary = page
    .locator("summary")
    .filter({ hasText: /repaired 1 query/ })
  await expect(summary).toBeVisible()
  await summary.click()
  await expect(
    page.getByRole("list", { name: "Progress" }).getByText("Repairing")
  ).toBeVisible()
})

test("explains a failed request", async ({ page }) => {
  await page.goto("/")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(neverSavesPrompt)
  await page.getByRole("button", { name: "Send" }).click()

  await expect(page).toHaveURL(requestPath)
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("without saving", { timeout: 90_000 })
  await expect(alert.getByRole("button", { name: "Try again" })).toBeVisible()
  await page
    .locator("summary")
    .filter({ hasText: /^Ran \d+ steps?/ })
    .click()
  await expect(
    page.getByRole("list", { name: "Progress" }).getByText("Running queries")
  ).not.toHaveCount(0)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    neverSavesPrompt
  )

  const failedUrl = page.url()
  await alert.getByRole("button", { name: "Try again" }).click()
  await expect(page).not.toHaveURL(failedUrl)
  await expect(page).toHaveURL(requestPath)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    neverSavesPrompt
  )
  await expect(alert).toContainText("without saving", { timeout: 90_000 })
})

test("retries a failed follow-up on the same dashboard", async ({ page }) => {
  const { id: dashboardId } = await createDashboard(page.request, {
    ...loadSpec("overview"),
    title: uniqueTitle("Follow-up retry"),
  })
  const queued = await page.request.post("/api/v1/observe", {
    data: { prompt: unanswerablePrompt, dashboard_id: dashboardId },
    headers: mutationHeaders,
  })
  expect(queued.status()).toBe(202)
  const { request } = (await queued.json()) as { request: { id: string } }

  await page.goto(`/observe/${request.id}`)
  const alerts = page.getByRole("alert")
  await expect(alerts).toContainText("no replay fixture", { timeout: 30_000 })
  await expect(alerts).toContainText("could not be updated")
  const open = alerts.getByRole("link", openLink)
  await expect(open).toHaveAttribute("href", `/dashboards/${dashboardId}`)
  await expect(open).toHaveAttribute("target", "_blank")
  await alerts.getByRole("button", { name: "Try again" }).click()

  await expect(alerts).toHaveCount(2, { timeout: 30_000 })
  await expect(page).toHaveURL(new RegExp(`/observe/${request.id}$`))
  await expect(alerts.last()).toContainText("no replay fixture", {
    timeout: 30_000,
  })

  const revisions = await page.request.get(
    `/api/v1/dashboards/${dashboardId}/revisions`
  )
  const body = (await revisions.json()) as {
    revisions: { source: string; prompt: string }[]
  }
  expect(body.revisions).toHaveLength(2)
  for (const revision of body.revisions) {
    expect(revision).toMatchObject({
      source: "prompt",
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
  const commented = await page.request.post(
    `/api/v1/dashboards/${dashboardId}/comments`,
    {
      data: {
        version: 1,
        panel_id: "rps",
        body: unanswerablePrompt,
        request_revision: true,
      },
      headers: mutationHeaders,
    }
  )
  expect(commented.status()).toBe(201)
  const { comment } = (await commented.json()) as {
    comment: { revision: { id: string } | null }
  }
  expect(comment.revision).not.toBeNull()

  await page.goto(`/observe/${comment.revision?.id}`)
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("no replay fixture", { timeout: 30_000 })
  await expect(alert.getByRole("button", { name: "Try again" })).toHaveCount(0)
  const open = alert.getByRole("link", openLink)
  await expect(open).toHaveAttribute("href", `/dashboards/${dashboardId}`)
  await expect(open).toHaveAttribute("target", "_blank")
})

test("follows up in the same chat", async ({ page }) => {
  await page.goto("/")
  const promptBar = page.getByRole("textbox", { name: "Message the agent" })
  await promptBar.fill(redPrompt)
  await promptBar.press("Enter")

  await expect(page).toHaveURL(requestPath)
  const conversationUrl = page.url()
  await expect(page.getByText("Saved as version 1")).toBeVisible({
    timeout: 90_000,
  })

  await promptBar.fill(followUpPrompt)
  await promptBar.press("Enter")
  await expect(page).toHaveURL(conversationUrl)
  await expect(page.getByText(followUpPrompt, { exact: true })).toBeVisible()
  await expect(page.getByText("Saved as version 2")).toBeVisible({
    timeout: 90_000,
  })
  await expect(page).toHaveURL(conversationUrl)
  await expect(promptBar).toHaveAttribute(
    "placeholder",
    "Ask for another change to Demo shop RED metrics"
  )
})

test("slash focuses the chat composer", async ({ page }) => {
  await page.goto("/")
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
    let openGate!: () => void
    const gate = new Promise<void>((resolve) => (openGate = resolve))
    await page.route(requestApi, async (route) => {
      await gate
      await route.continue()
    })

    await page.goto("/")
    const promptBar = page.getByRole("textbox", { name: "Message the agent" })
    await expect(promptBar).toHaveAttribute("enterkeyhint", "enter")
    await expect(promptBar).not.toHaveAttribute("aria-describedby")
    await expect(page.getByText("Enter sends")).toHaveCount(0)

    await promptBar.fill(redPrompt)
    await promptBar.press("Enter")
    await expect(promptBar).toHaveValue(`${redPrompt}\n`)
    await expect(page).toHaveURL(/\/$/)

    await page.getByRole("button", { name: "Send" }).click()
    await expect(page).toHaveURL(requestPath)
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(redPrompt)
    openGate()

    await expect(page.getByRole("link", openLink)).toBeVisible({
      timeout: 90_000,
    })
  })
})
