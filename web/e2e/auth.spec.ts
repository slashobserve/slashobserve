import { signInThroughDex } from "./dex"
import { expect, test } from "./fixtures"

test("shows the signed-in user", async ({ page }) => {
  await page.goto("/")
  await expect(
    page
      .getByRole("navigation", { name: "Main" })
      .getByText("admin@example.com")
  ).toBeVisible()
  await expect(page.getByRole("button", { name: /^Account: / })).toBeVisible()
  await expect(
    page.getByRole("heading", { level: 1, name: "What do you want to see?" })
  ).toBeVisible()
})

test("shows the new user after the session changes in another tab", async ({
  page,
}) => {
  await page.goto("/")
  await expect(page.getByText("admin@example.com")).toBeVisible()

  await page.route("**/api/v1/auth/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        authenticated: true,
        principal: {
          kind: "user",
          id: "other-user",
          email: "other@example.com",
          name: "Other",
        },
      }),
    })
  )
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => "visible",
    })
    document.dispatchEvent(new Event("visibilitychange"))
  })

  await expect(page.getByText("other@example.com")).toBeVisible()
  await expect(page.getByText("admin@example.com")).toHaveCount(0)
})

test("falls back to the home page for an unsafe return_to", async ({
  page,
}) => {
  await page.goto("/login?return_to=/%5Cevil.com")
  await expect(page).toHaveURL("/")
  await expect(
    page.getByRole("heading", { level: 1, name: "What do you want to see?" })
  ).toBeVisible()
})

test("keeps the page layout on the root error screen", async ({
  page,
  consoleErrors,
}) => {
  await page.route("**/api/v1/auth/session", (route) =>
    route.fulfill({ status: 500, contentType: "application/json", body: "{}" })
  )
  await page.goto("/")
  const heading = page.getByRole("heading", { name: "Something went wrong" })
  await expect(heading).toBeVisible()
  await expect(
    page
      .locator("div.min-h-svh.bg-background.text-foreground")
      .filter({ has: heading })
  ).toBeVisible()
  expect(consoleErrors.length).toBeGreaterThan(0)
  consoleErrors.length = 0
})

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

  test("explains a failed sign-in", async ({ page }) => {
    await page.goto("/login?error=email_not_allowed")
    await expect(page.getByRole("alert")).toHaveText(
      "This account is not allowed to sign in."
    )
  })

  test("uses the generic message for an inherited error code", async ({
    page,
  }) => {
    await page.goto("/login?error=constructor")
    await expect(page.getByRole("alert")).toHaveText(
      "Sign-in failed. Try again."
    )
  })

  test("still renders the login page when the session check fails", async ({
    page,
  }) => {
    await page.route("**/api/v1/auth/session", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: "{" })
    )
    await page.goto("/login")
    await expect(page.getByRole("alert")).toHaveText(
      "SlashObserve is unavailable. Try again shortly."
    )
    await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible()
  })
})
