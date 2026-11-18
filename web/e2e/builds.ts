import { expect, type Page } from "@playwright/test"

const requestApi = /\/api\/v1\/observe\/[0-9a-f-]{36}$/

async function finishedOnServer(page: Page, requestId: string) {
  await expect
    .poll(
      async () => {
        const response = await page.request.get(`/api/v1/observe/${requestId}`)
        const body = (await response.json()) as { request: { status: string } }
        return body.request.status
      },
      { timeout: 90_000, intervals: [250] }
    )
    .toBe("succeeded")
}

export async function holdRequestPollsUntilFinished(page: Page) {
  let open!: () => void
  const gate = new Promise<void>((resolve) => (open = resolve))
  await page.route(requestApi, async (route) => {
    await gate
    await route.continue()
  })
  return async (requestId: string) => {
    await finishedOnServer(page, requestId)
    open()
  }
}

const requestPage = /\/[bc]\/([0-9a-f-]{36})$/
const dashboardPage = /\/d\/[0-9a-f-]{36}/

export async function sentRequestId(page: Page): Promise<string> {
  await expect(page).toHaveURL(requestPage)
  return requestPage.exec(page.url())?.[1] ?? ""
}

export async function reachDashboard(page: Page, timeout = 90_000) {
  const open = page.getByRole("link", { name: "Open dashboard", exact: true })
  await expect
    .poll(
      async () =>
        dashboardPage.test(page.url()) || (await open.first().isVisible()),
      { timeout }
    )
    .toBe(true)
  if (!dashboardPage.test(page.url())) await open.first().click()
  await expect(page).toHaveURL(dashboardPage)
}
