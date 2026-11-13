import { expect, test } from "./fixtures"

test("loads the app shell", async ({ page }) => {
  const response = await page.goto("/")
  expect(response?.status()).toBe(200)
  await expect(page).toHaveURL(/^https?:\/\/[^/]+\/$/)
  await expect(page).toHaveTitle("Home - SlashObserve")
  await expect(
    page.getByRole("heading", { level: 1, name: "What are you working on?" })
  ).toBeVisible()
})

test("serves deep links through the SPA fallback", async ({ page }) => {
  const response = await page.goto("/no/such/page")
  expect(response?.status()).toBe(200)
  await expect(
    page.getByRole("heading", { name: "Page not found" })
  ).toBeVisible()
  await page.getByRole("link", { name: "Back to home" }).click()
  await expect(page).toHaveURL(/^https?:\/\/[^/]+\/$/)
  await expect(
    page.getByRole("heading", { level: 1, name: "What are you working on?" })
  ).toBeVisible()
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("hides the sidebar and keeps the phone bar and top row free of clipped text", async ({
    page,
  }) => {
    await page.goto("/")
    await expect(
      page.getByRole("heading", { level: 1, name: "What are you working on?" })
    ).toBeVisible()
    await expect(page.getByRole("button", { name: "Menu" })).toBeVisible()
    await expect(page.getByRole("navigation", { name: "Main" })).toBeHidden()
    const headers = page.locator("main > header")
    await expect(headers).toHaveCount(2)
    const clipped = await headers.evaluateAll(
      (elements) =>
        elements.flatMap((element) =>
          [...element.querySelectorAll("*")].filter(
            (item) =>
              item instanceof HTMLElement &&
              item.offsetWidth > 1 &&
              item.scrollWidth > item.clientWidth
          )
        ).length
    )
    expect(clipped).toBe(0)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    ).toBe(true)
  })
})
