import { createDashboard, loadSpec, mutationHeaders, uniqueTitle } from "./api"
import { expectPanelsReady } from "./dashboards"
import { expect, test } from "./fixtures"

const redPrompt = "demo shop RED metrics by endpoint"
const neverSavesPrompt = "e2e: a slow run that never saves"
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"
const requestPath = /\/observe\/([0-9a-f-]{36})$/
const dashboardPath = /\/dashboards\/([0-9a-f-]{36})/
const requestApi = /\/api\/v1\/observe\/[0-9a-f-]{36}$/

type ObserveBody = {
  request: {
    status: string
    dashboard_id: string | null
    steps: { phase: string }[]
  }
}

test("creates a dashboard from a prompt", async ({ page }) => {
  let openGate!: () => void
  const gate = new Promise<void>((resolve) => (openGate = resolve))
  await page.route(requestApi, async (route) => {
    await gate
    await route.continue()
  })

  await page.goto("/")
  const promptBar = page.getByRole("textbox", {
    name: "Describe the dashboard you want",
  })
  await promptBar.fill(redPrompt)
  await promptBar.press("Enter")

  await expect(page).toHaveURL(requestPath)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(redPrompt)
  await expect(
    page.getByRole("list", { name: "Progress" }).getByText("Working")
  ).toBeVisible()
  openGate()

  await expect(page).toHaveURL(dashboardPath, { timeout: 90_000 })
  await expectPanelsReady(page)
  await expect(page.getByText("No data")).toHaveCount(0)
  await expect(page.getByRole("alert")).toHaveCount(0)

  const dashboardId = dashboardPath.exec(page.url())?.[1]
  expect(dashboardId).toBeDefined()

  await page.goBack()
  await expect(page).toHaveURL(/\/$/)
  await expect(
    page.locator(`a[href="/dashboards/${dashboardId}"]`)
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
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${finished?.dashboard_id}`)
  )
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
})

test("explains a failed request", async ({ page }) => {
  await page.goto("/")
  const promptBar = page.getByRole("textbox", {
    name: "Describe the dashboard you want",
  })
  await promptBar.fill(neverSavesPrompt)
  await page.getByRole("button", { name: "Observe" }).click()

  await expect(page).toHaveURL(requestPath)
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("without saving", { timeout: 90_000 })
  await expect(alert.getByRole("button", { name: "Try again" })).toBeVisible()
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
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("no replay fixture", { timeout: 30_000 })
  await expect(alert).toContainText("could not be updated")
  await expect(
    alert.getByRole("link", { name: "Back to the dashboard" })
  ).toHaveAttribute("href", `/dashboards/${dashboardId}`)
  await alert.getByRole("button", { name: "Try again" }).click()

  await expect(page).not.toHaveURL(new RegExp(request.id))
  const retriedId = requestPath.exec(page.url())?.[1]
  expect(retriedId).toBeDefined()
  const retried = await page.request.get(`/api/v1/observe/${retriedId}`)
  const body = (await retried.json()) as {
    request: { source: string; dashboard_id: string | null; prompt: string }
  }
  expect(body.request).toMatchObject({
    source: "prompt",
    dashboard_id: dashboardId,
    prompt: unanswerablePrompt,
  })
  await expect(alert).toContainText("no replay fixture", { timeout: 30_000 })
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
      data: { version: 1, panel_id: "rps", body: unanswerablePrompt },
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
  await expect(
    alert.getByRole("link", { name: "Back to the dashboard" })
  ).toHaveAttribute("href", `/dashboards/${dashboardId}`)
})

test("slash focuses the prompt bar", async ({ page }) => {
  await page.goto("/")
  const promptBar = page.getByRole("textbox", {
    name: "Describe the dashboard you want",
  })
  await expect(promptBar).toBeVisible()
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
    await page.goto("/")
    const promptBar = page.getByRole("textbox", {
      name: "Describe the dashboard you want",
    })
    await expect(promptBar).toHaveAttribute("enterkeyhint", "enter")
    await expect(promptBar).not.toHaveAttribute("aria-describedby")
    await expect(page.getByText("Enter sends")).toHaveCount(0)

    await promptBar.fill(redPrompt)
    await promptBar.press("Enter")
    await expect(promptBar).toHaveValue(`${redPrompt}\n`)
    await expect(page).toHaveURL(/\/$/)

    await page.getByRole("button", { name: "Observe" }).click()
    await expect(page).toHaveURL(requestPath)
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(redPrompt)
  })
})
