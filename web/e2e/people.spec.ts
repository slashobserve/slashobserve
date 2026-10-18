import type { Route } from "@playwright/test"

import { expect, test, type Page } from "./fixtures"

type Person = {
  id: string
  name: string
  email: string
  created_at: string
  last_login_at: string
  last_seen_at: string
  dashboard_count: number
  key_count: number
  session_count: number
}

const priyaId = "7b1c3f2e-4d5a-4e6f-8a9b-0c1d2e3f4a5b"

function daysAgo(days: number): string {
  return new Date(Date.now() - days * 86_400_000).toISOString()
}

function priya(): Person {
  const seen = daysAgo(21)
  return {
    id: priyaId,
    name: "Priya",
    email: "priya@example.com",
    created_at: daysAgo(60),
    last_login_at: seen,
    last_seen_at: seen,
    dashboard_count: 0,
    key_count: 0,
    session_count: 1,
  }
}

async function addPeople(page: Page, extra: Person[]) {
  await page.route(
    (url) => url.pathname === "/api/v1/users",
    async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback()
        return
      }
      const response = await route.fetch()
      const body = (await response.json()) as { users: Person[] }
      await route.fulfill({
        response,
        json: { users: [...body.users, ...extra], next_cursor: null },
      })
    }
  )
}

async function mockSignOut(
  page: Page,
  id: string,
  respond: (route: Route) => Promise<void>
): Promise<string[]> {
  const sent: string[] = []
  const pattern = new RegExp(`/api/v1/users/${id}/sessions$`)
  await page.route(
    (url) => pattern.test(url.pathname),
    async (route) => {
      sent.push(route.request().method())
      await respond(route)
    }
  )
  return sent
}

async function adminId(page: Page): Promise<string> {
  const response = await page.request.get("/api/v1/auth/session")
  const session = (await response.json()) as { principal: { id: string } }
  return session.principal.id
}

async function openPeople(page: Page) {
  await page.goto("/people")
  await expect(
    page.getByRole("heading", { level: 1, name: "People" })
  ).toBeVisible()
  await expect(page.locator("tbody tr").first()).toBeVisible()
}

function row(page: Page, email: string) {
  return page.locator("tbody tr", { hasText: email })
}

function failedResourceLogged(consoleErrors: string[], status: string) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(logged[0]), 1)
}

async function noDocumentOverflow(page: Page) {
  return page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth
  )
}

test("lists the signed-in user as online", async ({ page }) => {
  await openPeople(page)
  const admin = row(page, "admin@example.com")
  await expect(admin.locator('[data-status="ok"]')).toHaveText("Online now")
  const cells = admin.locator("td")
  await expect(cells.nth(2)).toHaveText(/^\d+$/)
  await expect(cells.nth(3)).toHaveText(/^\d+$/)
})

test("states who can sign in", async ({ page }) => {
  await openPeople(page)
  await expect(
    page.getByText("Anyone with a verified email at", { exact: false })
  ).toHaveText(
    "Anyone with a verified email at example.com can sign in through Dex."
  )
  await page.getByRole("link", { name: "Sign-in settings" }).click()
  await expect(page).toHaveURL(/\/settings$/)
})

test("signs another person out everywhere", async ({ page }) => {
  await addPeople(page, [priya()])
  const sent = await mockSignOut(page, priyaId, (route) =>
    route.fulfill({ status: 204 })
  )
  await openPeople(page)

  const rows = page.locator("tbody tr")
  await expect(rows.first()).toContainText("admin@example.com")
  await expect(rows.last()).toContainText("priya@example.com")
  await expect(row(page, "priya@example.com")).toContainText("3 weeks ago")

  const trigger = page.getByRole("button", {
    name: "Sign out Priya everywhere",
  })
  await trigger.click()
  const dialog = page.getByRole("alertdialog")
  await expect(
    dialog.getByRole("heading", { name: "Sign out Priya everywhere?" })
  ).toBeVisible()
  await expect(dialog).not.toContainText("This includes this browser.")
  await dialog
    .getByRole("button", { name: "Sign out everywhere", exact: true })
    .click()

  await expect(page.getByText("Signed out Priya everywhere")).toBeVisible()
  await expect(dialog).toBeHidden()
  expect(sent).toEqual(["DELETE"])
  await expect(trigger).toBeFocused()
})

test("signs yourself out everywhere", async ({ page }) => {
  const id = await adminId(page)
  let signedOut = false
  await page.route("**/api/v1/auth/session", async (route) => {
    if (!signedOut) {
      await route.fallback()
      return
    }
    await route.fulfill({ json: { authenticated: false } })
  })
  const sent = await mockSignOut(page, id, async (route) => {
    signedOut = true
    await route.fulfill({ status: 204 })
  })
  await openPeople(page)

  await page
    .getByRole("button", { name: /^Sign out .+ everywhere$/ })
    .first()
    .click()
  const dialog = page.getByRole("alertdialog")
  await expect(dialog).toContainText("This includes this browser.")
  await dialog
    .getByRole("button", { name: "Sign out everywhere", exact: true })
    .click()

  await expect(page).toHaveURL(/\/login/)
  expect(sent).toEqual(["DELETE"])
})

test("reports a failure", async ({ page, consoleErrors }) => {
  await addPeople(page, [priya()])
  await mockSignOut(page, priyaId, (route) =>
    route.fulfill({
      status: 404,
      json: { error: "person not found", code: "not_found" },
    })
  )
  await openPeople(page)

  await page.getByRole("button", { name: "Sign out Priya everywhere" }).click()
  const dialog = page.getByRole("alertdialog")
  await dialog
    .getByRole("button", { name: "Sign out everywhere", exact: true })
    .click()

  await expect(dialog.getByRole("alert")).toHaveText(
    "This person no longer exists."
  )
  failedResourceLogged(consoleErrors, "404")
})

test("shows allow-all sign-in", async ({ page }) => {
  await page.route(
    (url) => url.pathname === "/api/v1/instance",
    async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as Record<string, unknown>
      await route.fulfill({
        response,
        json: { ...body, allowed_email_domains: [], any_email_domain: true },
      })
    }
  )
  await openPeople(page)
  const banner = page.locator('[data-status="warning"]')
  await expect(banner).toBeVisible()
  await expect(
    page.getByText("Anyone with a verified email can sign in through")
  ).toHaveText("Anyone with a verified email can sign in through Dex.")
})

test("works with the keyboard", async ({ page }) => {
  await openPeople(page)
  const trigger = page
    .getByRole("button", { name: /^Sign out .+ everywhere$/ })
    .first()
  for (let presses = 0; presses < 80; presses++) {
    if (await trigger.evaluate((el) => el === document.activeElement)) break
    await page.keyboard.press("Tab")
  }
  await expect(trigger).toBeFocused()

  await page.keyboard.press("Enter")
  const dialog = page.getByRole("alertdialog")
  await expect(dialog).toBeVisible()
  const cancel = dialog.getByRole("button", { name: "Cancel" })
  for (let presses = 0; presses < 4; presses++) {
    if (await cancel.evaluate((el) => el === document.activeElement)) break
    await page.keyboard.press("Tab")
  }
  await expect(cancel).toBeFocused()

  await page.keyboard.press("Escape")
  await expect(dialog).toBeHidden()
  await expect(trigger).toBeFocused()
})

test("draws both themes and fits a phone", async ({ page }) => {
  const card = page.getByRole("region", { name: "People" })

  await page.emulateMedia({ colorScheme: "light" })
  await openPeople(page)
  await expect(card).toHaveCSS("background-color", "rgb(255, 255, 255)")

  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  await expect(card).toHaveCSS("background-color", "rgb(17, 17, 19)")

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(card).toBeVisible()
  expect(await noDocumentOverflow(page)).toBe(true)
})
