import type { Route } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expect, test, type Page } from "./fixtures"

const pageNames = [
  "Observe",
  "Dashboards",
  "Datasources",
  "API keys",
  "People",
  "Settings",
]

function trigger(page: Page) {
  return page.getByRole("button", { name: "Search or jump to" })
}

function palette(page: Page) {
  return page.getByRole("dialog", { name: "Search or jump to" })
}

function searchInput(page: Page) {
  return palette(page).getByRole("combobox", {
    name: "Search dashboards and pages",
  })
}

async function expectReadyToType(page: Page) {
  await expect(palette(page)).toBeVisible()
  await expect(searchInput(page)).toBeFocused()
}

async function openFromButton(page: Page) {
  await page.goto("/")
  await trigger(page).click()
  await expectReadyToType(page)
}

async function openFromShortcut(page: Page) {
  await page.goto("/")
  await expect(trigger(page)).toBeVisible()
  await page.keyboard.press("ControlOrMeta+k")
  await expectReadyToType(page)
}

async function expectEveryPage(page: Page) {
  for (const name of pageNames) {
    await expect(
      palette(page).getByRole("option", { name, exact: true })
    ).toBeVisible()
  }
}

function dropLoggedFailures(consoleErrors: string[], status: string) {
  for (let index = consoleErrors.length - 1; index >= 0; index--) {
    if (consoleErrors[index].includes(`status of ${status}`)) {
      consoleErrors.splice(index, 1)
    }
  }
}

test("opens from the button and lists the pages", async ({ page }) => {
  await openFromButton(page)
  await expectEveryPage(page)
  expect(
    await palette(page).getByRole("option").count()
  ).toBeGreaterThanOrEqual(pageNames.length)
  await page.keyboard.press("Escape")
  await expect(palette(page)).toHaveCount(0)
  await expect(trigger(page).locator("kbd")).toHaveText(/^(⌘K|Ctrl K)$/)
})

test("jumps to a page with the keyboard", async ({ page }) => {
  await openFromShortcut(page)
  await page.keyboard.type("api")
  await expect(
    palette(page).getByRole("option", { name: "API keys", exact: true })
  ).toHaveAttribute("data-highlighted", "")
  await page.keyboard.press("Enter")
  await expect(page).toHaveURL(/\/keys$/)
  await expect(palette(page)).toHaveCount(0)
})

test("opens a matching dashboard in a new tab", async ({ page, request }) => {
  const title = uniqueTitle("Palette")
  const { id } = await createDashboard(request, {
    ...loadSpec("overview"),
    title,
  })
  await openFromShortcut(page)
  const before = page.url()
  await page.keyboard.type(title)
  await expect(searchInput(page)).toHaveValue(title)
  const option = palette(page).getByRole("option", { name: title })
  await expect(option).toHaveAttribute("data-highlighted", "")
  const [popup] = await Promise.all([
    page.context().waitForEvent("page"),
    page.keyboard.press("Enter"),
  ])
  await expect(popup).toHaveURL(new RegExp(`/dashboards/${id}$`))
  await popup.close()
  expect(page.url()).toBe(before)
})

test("explains an empty result", async ({ page }) => {
  await openFromButton(page)
  await page.keyboard.type("zzzz-no-match")
  await expect(
    palette(page).getByText('No pages or dashboards match "zzzz-no-match".')
  ).toBeVisible()
})

test("closes with Escape and returns focus", async ({ page }) => {
  await openFromButton(page)
  await page.keyboard.press("Escape")
  await expect(palette(page)).toHaveCount(0)
  await expect(trigger(page)).toBeFocused()
})

test("keeps pages when dashboards fail to load", async ({
  page,
  consoleErrors,
}) => {
  const fail = (route: Route) =>
    route.request().method() === "GET"
      ? route.fulfill({
          status: 500,
          contentType: "application/json",
          body: '{"error":"x","code":"internal"}',
        })
      : route.fallback()
  await page.route((url) => url.pathname === "/api/v1/dashboards", fail)
  await openFromButton(page)
  await expect(
    palette(page).getByText("Could not load dashboards.")
  ).toBeVisible({ timeout: 15_000 })
  await expectEveryPage(page)
  dropLoggedFailures(consoleErrors, "500")
})

test("draws the palette flat in dark mode", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await openFromButton(page)
  const styles = await palette(page).evaluate((element) => {
    const computed = getComputedStyle(element)
    return {
      boxShadow: computed.boxShadow,
      backgroundColor: computed.backgroundColor,
      borderTopWidth: computed.borderTopWidth,
    }
  })
  expect(styles).toEqual({
    boxShadow: "none",
    backgroundColor: "rgb(17, 17, 19)",
    borderTopWidth: "1px",
  })
})

test("runs sign out from the palette", async ({ page, consoleErrors }) => {
  let signOutRequests = 0
  await page.route("**/api/v1/auth/logout", (route) => {
    signOutRequests++
    return route.fulfill({
      status: 500,
      contentType: "application/json",
      body: '{"error":"x","code":"internal"}',
    })
  })
  await openFromButton(page)
  await page.keyboard.type("sign out")
  await page.keyboard.press("Enter")
  await expect(palette(page)).toHaveCount(0)
  await expect(page.getByText("Sign out failed. Try again.")).toBeVisible()
  expect(signOutRequests).toBe(1)
  await expect(page).toHaveURL(/\/$/)
  dropLoggedFailures(consoleErrors, "500")
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("navigates and offers sign out without the sidebar", async ({
    page,
  }) => {
    await page.goto("/")
    await expect(page.getByRole("navigation", { name: "Main" })).toBeHidden()
    await expect(trigger(page)).toBeVisible()
    await trigger(page).click()
    await expect(palette(page)).toBeVisible()
    await expectEveryPage(page)
    await expect(
      palette(page).getByRole("option", { name: "Sign out", exact: true })
    ).toBeVisible()

    await palette(page)
      .getByRole("option", { name: "Settings", exact: true })
      .click()
    await expect(page).toHaveURL(/\/settings$/)
    await expect(palette(page)).toHaveCount(0)

    await trigger(page).click()
    await expectReadyToType(page)
    await page.keyboard.type("sign")
    await expect(
      palette(page).getByRole("option", { name: "Sign out", exact: true })
    ).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(palette(page)).toHaveCount(0)
  })
})
