import { expect, test as base, type Page } from "@playwright/test"

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

export function failedResourceLogged(
  consoleErrors: string[],
  status: string,
  count = 1
) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).toHaveLength(count)
  for (const message of logged) {
    consoleErrors.splice(consoleErrors.indexOf(message), 1)
  }
}

export async function draftPreviewRejectionLogged(consoleErrors: string[]) {
  const rejections = () =>
    consoleErrors.filter((message) => message.includes("status of 422"))
  await expect.poll(() => rejections().length).toBe(1)
  const [rejection] = rejections()
  if (rejection === undefined) throw new Error("no draft preview rejection")
  consoleErrors.splice(consoleErrors.indexOf(rejection), 1)
}

export { expect, type Page }
