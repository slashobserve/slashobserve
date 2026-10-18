import type { APIRequestContext, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard } from "./dashboards"
import { expect, test } from "./fixtures"

async function createPrimitivesDashboard(request: APIRequestContext) {
  const { id } = await createDashboard(request, {
    ...loadSpec("overview"),
    title: uniqueTitle("Primitives"),
  })
  return id
}

function firstPanelMenu(page: Page) {
  return page.getByRole("button", { name: /^Panel menu: / }).first()
}

async function expectFlatMenu(page: Page) {
  await firstPanelMenu(page).click()
  const menu = page.getByRole("menu")
  await expect(menu).toBeVisible()
  const style = await menu.evaluate((element) => {
    const computed = getComputedStyle(element)
    return {
      boxShadow: computed.boxShadow,
      borderTopWidth: computed.borderTopWidth,
    }
  })
  expect(style.boxShadow).toBe("none")
  expect(style.borderTopWidth).toBe("1px")
}

test("menus are flat with a border and focus is a solid brand ring", async ({
  page,
  request,
}) => {
  await page.emulateMedia({ colorScheme: "light" })
  const id = await createPrimitivesDashboard(request)
  await openDashboard(page, id)

  await expectFlatMenu(page)
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menu")).toHaveCount(0)

  const menuButton = firstPanelMenu(page)
  await menuButton.focus()
  await expect(menuButton).toBeFocused()
  await page.keyboard.press("Shift+Tab")
  await page.keyboard.press("Tab")
  await expect(menuButton).toBeFocused()
  await expect
    .poll(() => menuButton.evaluate((el) => getComputedStyle(el).boxShadow))
    .toContain("42, 120, 214")
})

test("header buttons use the 6px radius and 13px text", async ({
  page,
  request,
}) => {
  const id = await createPrimitivesDashboard(request)
  await openDashboard(page, id)

  const signOut = page.getByRole("button", { name: "Sign out" })
  await expect(signOut).toBeVisible()
  const style = await signOut.evaluate((element) => {
    const computed = getComputedStyle(element)
    return { borderRadius: computed.borderRadius, fontSize: computed.fontSize }
  })
  expect(style.borderRadius).toBe("6px")
  expect(style.fontSize).toBe("13px")
})

test("menus stay flat in dark mode", async ({ page, request }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  const id = await createPrimitivesDashboard(request)
  await openDashboard(page, id)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await expectFlatMenu(page)
})
