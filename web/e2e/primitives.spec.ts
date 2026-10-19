import type { APIRequestContext, Page } from "@playwright/test"

import {
  createDashboard,
  createServiceAccount,
  createServiceAccountKey,
  loadSpec,
  uniqueAccountName,
  uniqueTitle,
} from "./api"
import { expectPanelsReady, openDashboard } from "./dashboards"
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

  const more = page.getByRole("button", { name: /^More/ })
  await expect(more).toBeVisible()
  const radius = await more.evaluate(
    (element) => getComputedStyle(element).borderRadius
  )
  expect(radius).toBe("6px")
  const timeRange = page.getByRole("button", { name: /^Time range/ })
  const fontSize = await timeRange.evaluate(
    (element) => getComputedStyle(element).fontSize
  )
  expect(fontSize).toBe("13px")
})

test("menus stay flat in dark mode", async ({ page, request }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  const id = await createPrimitivesDashboard(request)
  await openDashboard(page, id)
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await expectFlatMenu(page)
})

test("a focused multi-value variable control has a 2px brand ring", async ({
  page,
  request,
}) => {
  await page.emulateMedia({ colorScheme: "light" })
  const id = await createPrimitivesDashboard(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const input = page.getByRole("combobox", { name: "Pod" })
  await expect(input).toBeEnabled()
  const chips = page
    .locator('[data-slot="combobox-chips"]')
    .filter({ has: input })
  await input.focus()
  await expect(input).toBeFocused()
  await expect
    .poll(() => chips.evaluate((el) => getComputedStyle(el).boxShadow))
    .toContain("42, 120, 214")
  const shadow = await chips.evaluate((el) => getComputedStyle(el).boxShadow)
  expect(shadow).toContain("0px 0px 0px 2px")
})

test("a destructive button focused by keyboard shows the brand ring", async ({
  page,
  request,
}) => {
  await page.emulateMedia({ colorScheme: "light" })
  const account = await createServiceAccount(request, uniqueAccountName())
  await createServiceAccountKey(request, account.id, "ci")
  await page.goto("/settings/keys")
  await page
    .getByRole("button", { name: `Revoke key ci for ${account.name}` })
    .click()
  const confirm = page.getByRole("alertdialog", { name: "Revoke key ci?" })
  const revoke = confirm.getByRole("button", {
    name: "Revoke key",
    exact: true,
  })
  await revoke.focus()
  await page.keyboard.press("Shift+Tab")
  await page.keyboard.press("Tab")
  await expect(revoke).toBeFocused()
  await expect
    .poll(() => revoke.evaluate((el) => getComputedStyle(el).boxShadow))
    .toContain("42, 120, 214")
})
