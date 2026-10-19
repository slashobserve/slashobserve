import type { Locator, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard } from "./dashboards"
import { expect, tabUntilFocused, test } from "./fixtures"

function sidebar(page: Page) {
  return page.getByRole("navigation", { name: "Main" })
}

function grip(page: Page) {
  return page.getByRole("separator", { name: "Resize sidebar" })
}

async function rect(locator: Locator) {
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  return box as NonNullable<typeof box>
}

async function expectWidth(locator: Locator, expected: number) {
  await expect
    .poll(async () => Math.abs((await rect(locator)).width - expected))
    .toBeLessThanOrEqual(1)
}

async function noHorizontalOverflow(page: Page) {
  return page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth
  )
}

const pages = [
  ["/", "Observe", "Dashboards"],
  ["/dashboards", "Dashboards", "Dashboards"],
  ["/datasources", "Datasources", "Datasources"],
  ["/keys", "API keys", "API keys"],
  ["/people", "People", "People"],
  ["/settings", "Settings", "Settings"],
] as const

test("renders every shell page with its crumb and current item", async ({
  page,
}) => {
  for (const [path, label, heading] of pages) {
    await page.goto(path)
    await expect(
      page.getByRole("heading", { level: 1, name: heading })
    ).toBeVisible()
    await expect(
      page
        .getByRole("navigation", { name: "Breadcrumb" })
        .locator('[aria-current="page"]')
    ).toHaveText(label)
    const current = sidebar(page).locator('a[aria-current="page"]')
    await expect(current).toHaveCount(1)
    await expect(current).toHaveAccessibleName(label)
  }
})

test("redirects /settings/keys to /keys", async ({ page }) => {
  await page.goto("/settings/keys")
  await expect(page).toHaveURL(/\/keys$/)
})

test("shows the mark, wordmark and server host", async ({ page }) => {
  await page.goto("/")
  const home = page.getByRole("link", { name: "/observe home" })
  await expect(home).toBeVisible()
  await expect(home).toContainText("/observe")
  const baseURL = test.info().project.use.baseURL as string
  await expect(home).toContainText(new URL(baseURL).host)
})

test("resizes the sidebar from the keyboard and remembers it", async ({
  page,
}) => {
  await page.goto("/")
  const handle = grip(page)
  await expect(handle).toHaveAttribute("aria-valuenow", "240")

  await handle.focus()
  await page.keyboard.press("ArrowRight")
  await expect(handle).toHaveAttribute("aria-valuenow", "248")
  await page.keyboard.press("End")
  await expect(handle).toHaveAttribute("aria-valuenow", "360")
  await expectWidth(sidebar(page), 360)
  await page.keyboard.press("Home")
  await expect(handle).toHaveAttribute("aria-valuenow", "200")

  await page.reload()
  await expect(grip(page)).toHaveAttribute("aria-valuenow", "200")

  await grip(page).dblclick()
  await expect(grip(page)).toHaveAttribute("aria-valuenow", "240")
})

test("resizes the sidebar by dragging", async ({ page }) => {
  await page.goto("/")
  const handle = grip(page)
  await expect(handle).toHaveAttribute("aria-valuenow", "240")
  const box = await rect(handle)
  const centerX = box.x + box.width / 2
  const centerY = box.y + box.height / 2

  await page.mouse.move(centerX, centerY)
  await page.mouse.down()
  await page.mouse.move(centerX + 60, centerY, { steps: 5 })
  await page.mouse.up()

  await expect(handle).toHaveAttribute("aria-valuenow", "300")
  await expectWidth(sidebar(page), 300)
})

test("collapses to icons with tooltips and remembers it", async ({ page }) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Collapse sidebar" }).click()
  await expectWidth(sidebar(page), 44)
  await expect(grip(page)).toHaveCount(0)

  const datasources = page.getByRole("link", { name: "Datasources" })
  await expect(datasources).toBeVisible()
  await datasources.hover()
  await expect(page.getByRole("tooltip")).toHaveText("Datasources")

  await page.reload()
  await expectWidth(sidebar(page), 44)
  await page.getByRole("button", { name: "Expand sidebar" }).click()
  await expectWidth(sidebar(page), 240)
})

test("hides the sidebar under 760px", async ({ page }) => {
  await page.goto("/")
  await page.setViewportSize({ width: 700, height: 900 })
  await expect(sidebar(page)).toBeHidden()
  await expect(
    page.getByRole("button", { name: "Collapse sidebar" })
  ).toBeHidden()
  await expect(page.getByRole("main")).toBeVisible()
  expect(await noHorizontalOverflow(page)).toBe(true)

  await page.setViewportSize({ width: 761, height: 900 })
  await expect(sidebar(page)).toBeVisible()
})

test("hides an open collapsed-sidebar tooltip under 760px", async ({
  page,
}) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Collapse sidebar" }).click()
  await page.getByRole("link", { name: "Datasources" }).hover()
  await expect(page.getByRole("tooltip")).toHaveText("Datasources")

  await page.setViewportSize({ width: 700, height: 900 })
  await expect(sidebar(page)).toBeHidden()
  await expect(page.getByRole("tooltip")).toBeHidden()
})

test("opens the account menu and returns focus", async ({ page }) => {
  await page.goto("/")
  const account = page.getByRole("button", { name: /^Account: / })
  await account.click()
  await expect(page.getByRole("menuitem", { name: "Sign out" })).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menuitem", { name: "Sign out" })).toHaveCount(0)
  await expect(account).toBeFocused()
})

test("keeps the dashboard outside the shell", async ({ page, request }) => {
  const { id } = await createDashboard(request, {
    ...loadSpec("overview"),
    title: uniqueTitle("Shell"),
  })
  await openDashboard(page, id)
  await expect(sidebar(page)).toHaveCount(0)
  await expect(
    page.getByRole("banner").getByRole("link", { name: "API keys" })
  ).toHaveAttribute("href", "/keys")
})

test("shows focus in brand blue", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")
  const datasources = sidebar(page).getByRole("link", { name: "Datasources" })
  await tabUntilFocused(page, datasources, 15)
  await expect
    .poll(() => datasources.evaluate((el) => getComputedStyle(el).boxShadow))
    .toContain("42, 120, 214")
})

test("draws the shell in dark mode", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await page.goto("/")
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)
  const main = page.getByRole("main")
  await expect(main).toBeVisible()

  const styles = await main.evaluate((element) => {
    const root = element.parentElement?.parentElement as HTMLElement
    const current = document.querySelector(
      'nav[aria-label="Main"] a[aria-current="page"]'
    ) as HTMLElement
    return {
      root: getComputedStyle(root).backgroundColor,
      main: getComputedStyle(element).backgroundColor,
      border: getComputedStyle(element).borderTopColor,
      current: getComputedStyle(current).backgroundColor,
    }
  })
  expect(styles).toEqual({
    root: "rgb(9, 9, 11)",
    main: "rgb(17, 17, 19)",
    border: "rgb(42, 42, 46)",
    current: "rgb(17, 17, 19)",
  })
})
