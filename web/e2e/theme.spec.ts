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

  await page.getByRole("button", { name: "Switch theme" }).click()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  expect(await background()).not.toBe(lightBackground)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(true)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await page.getByRole("button", { name: "Switch theme" }).click()
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(false)
  await expect(page.getByRole("button", { name: "Switch theme" })).toBeVisible()
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)
})

test("uses the M1 tokens in both themes", async ({ page }) => {
  const readTokens = () =>
    page.evaluate(() => {
      const root = getComputedStyle(document.documentElement)
      const body = getComputedStyle(document.body)
      return {
        background: body.backgroundColor,
        color: body.color,
        brand: root.getPropertyValue("--brand").trim(),
        series1: root.getPropertyValue("--series-1").trim(),
      }
    })

  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")
  expect(
    await page.evaluate(
      async () =>
        (await document.fonts.load('12px "Geist Mono Variable"')).length
    )
  ).toBeGreaterThan(0)
  expect(await readTokens()).toEqual({
    background: "rgb(255, 255, 255)",
    color: "rgb(10, 10, 10)",
    brand: "#2a78d6",
    series1: "#2a78d6",
  })

  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  expect(await readTokens()).toEqual({
    background: "rgb(17, 17, 19)",
    color: "rgb(250, 250, 250)",
    brand: "#5598e7",
    series1: "#3987e5",
  })
})

test("keeps plain search parameters", async ({ page }) => {
  await page.goto("/?from=now-1h&var-pod=a&var-pod=b")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
  await expect(page).toHaveURL(/\/\?from=now-1h&var-pod=a&var-pod=b$/)
})

test("switches the theme from the keyboard", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")
  const toggle = page.getByRole("button", { name: "Switch theme" })
  await toggle.focus()
  await page.keyboard.press("Enter")
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  await expect(toggle).toBeFocused()
})

test("matches the system theme from Settings", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await page.goto("/settings")
  const html = page.locator("html")
  await expect(html).toHaveClass(/\bdark\b/)
  const theme = page.getByRole("combobox", { name: "Theme" })
  await expect(theme).toContainText("Match the system")
  const stored = () =>
    page.evaluate(() => localStorage.getItem("slashobserve-theme"))

  await theme.click()
  await page.getByRole("option", { name: "Light", exact: true }).click()
  await expect(html).not.toHaveClass(/\bdark\b/)
  expect(await stored()).toBe("light")

  await theme.click()
  await page
    .getByRole("option", { name: "Match the system", exact: true })
    .click()
  await expect(html).toHaveClass(/\bdark\b/)
  expect(await stored()).toBeNull()

  await page.getByRole("button", { name: "Switch theme" }).click()
  await expect(html).not.toHaveClass(/\bdark\b/)
})
