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
