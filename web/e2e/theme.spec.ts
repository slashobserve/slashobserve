import { expect, switchTheme, test } from "./fixtures"

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

  await switchTheme(page)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  expect(await background()).not.toBe(lightBackground)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(true)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await switchTheme(page)
  await expect(page.locator("html")).not.toHaveClass(/\bdark\b/)

  await page.reload({ waitUntil: "domcontentloaded" })
  expect(await isDark()).toBe(false)
  await expect(page.getByRole("button", { name: /^Account: / })).toBeVisible()
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
        ring: root.getPropertyValue("--ring").trim(),
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
    color: "rgb(13, 13, 18)",
    ring: "#0a7f86",
    series1: "#2a78d6",
  })

  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  expect(await readTokens()).toEqual({
    background: "rgb(19, 20, 23)",
    color: "rgb(236, 236, 241)",
    ring: "#2ab3b1",
    series1: "#4a94ee",
  })
})

test("keeps plain search parameters", async ({ page }) => {
  await page.goto("/?from=now-1h&var-pod=a&var-pod=b")
  await expect(
    page.getByRole("heading", { level: 1, name: "What do you want to see?" })
  ).toBeVisible()
  await expect(page).toHaveURL(/\/ask\?from=now-1h&var-pod=a&var-pod=b$/)
})

test("switches the theme from the keyboard", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")
  const account = page.getByRole("button", { name: /^Account: / })
  await account.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("menuitem", { name: "Theme" })).toBeFocused()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByRole("menuitemradio", { name: "Light" })).toBeFocused()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("Enter")
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  await expect(account).toBeFocused()
})

test("matches the system theme from the account menu", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await page.goto("/settings")
  const html = page.locator("html")
  await expect(html).toHaveClass(/\bdark\b/)
  const account = page.getByRole("button", { name: /^Account: / })
  const choose = async (current: string, next: string) => {
    await account.click()
    await page.getByRole("menuitem", { name: "Theme" }).click()
    await expect(
      page.getByRole("menuitemradio", { name: current, exact: true })
    ).toBeChecked()
    await page.getByRole("menuitemradio", { name: next, exact: true }).click()
  }
  const stored = () =>
    page.evaluate(() => localStorage.getItem("slashobserve-theme"))

  await choose("Match the system", "Light")
  await expect(html).not.toHaveClass(/\bdark\b/)
  expect(await stored()).toBe("light")

  await choose("Light", "Match the system")
  await expect(html).toHaveClass(/\bdark\b/)
  expect(await stored()).toBeNull()

  await switchTheme(page)
  await expect(html).not.toHaveClass(/\bdark\b/)
})
