import { expect, test } from "./fixtures"

test("shows the signed-in user", async ({ page }) => {
  await page.goto("/")
  await expect(page.getByText("admin@example.com")).toBeVisible()
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible()
  await expect(
    page.getByRole("heading", { level: 1, name: "SlashObserve" })
  ).toBeVisible()
})

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test("redirects anonymous visitors to the login page", async ({ page }) => {
    await page.goto("/")
    await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
    await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible()
  })

  test("signs in through Dex and signs out", async ({ page }) => {
    await page.goto("/")
    await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
    await page.getByRole("link", { name: "Sign in" }).click()
    await page.locator('input[name="login"]').fill("admin@example.com")
    await page.locator('input[name="password"]').fill("password")
    await page.getByRole("button", { name: "Login" }).click()
    await expect(page).toHaveURL("/")
    await expect(page.getByText("admin@example.com")).toBeVisible()

    await page.getByRole("button", { name: "Sign out" }).click()
    await expect(page).toHaveURL("/login")

    await page.goto("/")
    await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
  })

  test("explains a failed sign-in", async ({ page }) => {
    await page.goto("/login?error=email_not_allowed")
    await expect(page.getByRole("alert")).toHaveText(
      "This account is not allowed to sign in."
    )
  })
})
