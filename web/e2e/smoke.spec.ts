import { expect, test } from "./fixtures"

test("loads the app shell", async ({ page }) => {
  const response = await page.goto("/")
  expect(response?.status()).toBe(200)
  await expect(page).toHaveTitle("SlashObserve")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
})

test("serves deep links through the SPA fallback", async ({ page }) => {
  const response = await page.goto("/no/such/page")
  expect(response?.status()).toBe(200)
  await expect(
    page.getByRole("heading", { name: "Page not found" })
  ).toBeVisible()
  await page.getByRole("link", { name: "Back to home" }).click()
  await expect(page).toHaveURL("/")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
})
