import { expect, test as base, type Locator, type Page } from "@playwright/test"

export const test = base.extend<{ consoleErrors: string[] }>({
  consoleErrors: [
    async ({ context }, use) => {
      const errors: string[] = []
      const watch = (page: Page) => {
        page.on("console", (message) => {
          if (message.type() === "error") errors.push(message.text())
        })
        page.on("pageerror", (error) => errors.push(error.message))
      }
      context.pages().forEach(watch)
      context.on("page", watch)
      await use(errors)
      expect(errors, "browser console errors").toEqual([])
    },
    { auto: true },
  ],
})

export function failedResourceLogged(consoleErrors: string[], status: string) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(logged[0]), 1)
}

const chunkFailure = /net::ERR_FAILED|dynamically imported module/

export function chunkFailureLogged(consoleErrors: string[]) {
  const logged = consoleErrors.filter((message) => chunkFailure.test(message))
  expect(logged.length).toBeGreaterThan(0)
  consoleErrors.splice(
    0,
    consoleErrors.length,
    ...consoleErrors.filter((message) => !chunkFailure.test(message))
  )
}

export async function tabUntilFocused(
  page: Page,
  locator: Locator,
  max: number
) {
  for (let presses = 0; presses < max; presses++) {
    if (await locator.evaluate((el) => el === document.activeElement)) break
    await page.keyboard.press("Tab")
  }
  await expect(locator).toBeFocused()
}

export { expect, type Page }
