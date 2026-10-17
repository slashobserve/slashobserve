import { expect, test } from "./fixtures"

test("follows the system theme by default", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await page.goto("/")
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await page.emulateMedia({ colorScheme: "light" })
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)
})

test("switches and remembers light and dark", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")
  const background = () =>
    page.evaluate(() => getComputedStyle(document.body).backgroundColor)
  const isDark = () =>
    page.evaluate(() => document.documentElement.classList.contains("dark"))
  const lightBackground = await background()

  await page.getByRole("button", { name: "Theme" }).click()
  await page.getByRole("menuitemradio", { name: "Dark" }).click()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  expect(await background()).not.toBe(lightBackground)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(true)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await page.getByRole("button", { name: "Theme" }).click()
  await page.getByRole("menuitemradio", { name: "Light" }).click()
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(false)
  await expect(page.getByRole("button", { name: "Theme" })).toBeVisible()
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)

  await page.getByRole("button", { name: "Theme" }).click()
  await page.getByRole("menuitemradio", { name: "System" }).click()
  await expect(page.getByRole("menuitemradio")).toHaveCount(0)
})

test("keeps plain search parameters", async ({ page }) => {
  await page.goto("/?from=now-1h&var-pod=a&var-pod=b")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
  await expect(page).toHaveURL(/\/\?from=now-1h&var-pod=a&var-pod=b$/)
})

test("opens the theme menu from the keyboard", async ({ page }) => {
  await page.goto("/")
  const trigger = page.getByRole("button", { name: "Theme" })
  await trigger.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("menuitemradio", { name: "Dark" })).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menuitemradio", { name: "Dark" })).toHaveCount(0)
  await expect(trigger).toBeFocused()
})
