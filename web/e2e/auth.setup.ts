import { authFile } from "./auth-file"
import { expect, test as setup } from "./fixtures"

setup("sign in through Dex", async ({ page }) => {
  await page.goto("/")
  await expect(page).toHaveURL(/\/login\?return_to=%2F$/)
  await page.getByRole("link", { name: "Sign in" }).click()
  await page.locator('input[name="login"]').fill("admin@example.com")
  await page.locator('input[name="password"]').fill("password")
  await page.getByRole("button", { name: "Login" }).click()
  await expect(page).toHaveURL("/")
  await expect(page.getByText("admin@example.com")).toBeVisible()
  await page.context().storageState({ path: authFile })
})
