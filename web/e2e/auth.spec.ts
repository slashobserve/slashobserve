import { signInThroughDex } from "./dex"
import { expect, test } from "./fixtures"

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test("redirects anonymous visitors to the login page", async ({ page }) => {
    await page.goto("/")
    await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
    await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible()
    await expect(page).toHaveTitle("Sign in - SlashObserve")
  })

  test("signs in through Dex and signs out", async ({ page }) => {
    await signInThroughDex(page)

    await page.getByRole("button", { name: /^Account: / }).click()
    await page.getByRole("menuitem", { name: "Sign out" }).click()
    await expect(page).toHaveURL("/login")

    await page.goto("/")
    await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
  })
})
