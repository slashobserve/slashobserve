import { expectPanelsReady } from "./dashboards"
import { expect, test } from "./fixtures"

const baseURL =
  process.env.SLASHOBSERVE_E2E_BASE_URL ?? "http://localhost:18080"

const redPrompt = "demo shop RED metrics by endpoint"
const neverSavesPrompt = "e2e: a slow run that never saves"
const requestPath = /\/observe\/[0-9a-f-]{36}$/
const dashboardPath = /\/dashboards\/[0-9a-f-]{36}/
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

  await page.goBack()
  await expect(page).toHaveURL(/\/$/)
  await expect(
    page.getByRole("link", { name: "Demo shop RED metrics" }).first()
  ).toBeVisible()
})

test("records the repair step and reopens finished requests", async ({
  page,
}) => {
  const created = await page.request.post("/api/v1/observe", {
    data: { prompt: redPrompt },
    headers: { Origin: baseURL },
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
