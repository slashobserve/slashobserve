import { readFileSync } from "node:fs"

import type { APIRequestContext, Page } from "@playwright/test"

import { createComment, createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { signInThroughDex } from "./dex"
import { expect, test } from "./fixtures"

const rpsTitle = "Checkout requests per second"

const themes = [
  {
    scheme: "light",
    background: "rgb(255, 255, 255)",
    border: "rgb(228, 228, 231)",
  },
  { scheme: "dark", background: "rgb(17, 17, 19)", border: "rgb(42, 42, 46)" },
] as const

async function createPageDashboard(request: APIRequestContext) {
  const title = uniqueTitle("Page")
  const { id } = await createDashboard(request, {
    ...loadSpec("overview"),
    title,
  })
  return { id, title }
}

function moreButton(page: Page) {
  return page.getByRole("button", { name: /^More/ })
}

async function noDocumentOverflow(page: Page) {
  return page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth
  )
}

for (const theme of themes) {
  test.describe(`in the ${theme.scheme} theme`, () => {
    test.beforeEach(async ({ page }) => {
      await page.emulateMedia({ colorScheme: theme.scheme })
    })

    test("shows the breadcrumb top row", async ({ page, request }) => {
      const { id, title } = await createPageDashboard(request)
      await openDashboard(page, id)

      const breadcrumb = page.getByRole("navigation", { name: "Breadcrumb" })
      await expect(
        breadcrumb.getByRole("link", { name: "/observe home" })
      ).toHaveAttribute("href", "/")
      await expect(
        breadcrumb.getByRole("link", { name: "Dashboards" })
      ).toHaveAttribute("href", "/dashboards")
      await expect(
        breadcrumb.getByRole("heading", { level: 1, name: title })
      ).toBeVisible()
      await expect(page.getByRole("navigation", { name: "Main" })).toHaveCount(
        0
      )
      await expect(page.getByText(/made by/)).toHaveCount(0)
    })

    test("draws flat panel cards", async ({ page, request }) => {
      const { id } = await createPageDashboard(request)
      await openDashboard(page, id)
      await expectPanelsReady(page)

      await expect(page.locator("body")).toHaveCSS(
        "background-color",
        theme.background
      )
      const cards = page.locator("article[data-panel-id]")
      const count = await cards.count()
      expect(count).toBeGreaterThan(0)
      for (let index = 0; index < count; index++) {
        const card = cards.nth(index)
        await expect(card).toHaveCSS("border-radius", "10px")
        await expect(card).toHaveCSS("box-shadow", "none")
        await expect(card).toHaveCSS("border-top-width", "1px")
        await expect(card).toHaveCSS("border-top-color", theme.border)
      }
      await expect(
        panel(page, rpsTitle).getByText("prom", { exact: true })
      ).toBeVisible()
    })
  })
}

test("exports the spec", async ({ page, request }) => {
  const { id, title } = await createPageDashboard(request)
  await openDashboard(page, id)

  await moreButton(page).click()
  const downloading = page.waitForEvent("download")
  await page.getByRole("menuitem", { name: "Export spec JSON" }).click()
  const download = await downloading

  expect(download.suggestedFilename()).toMatch(/^page-[a-z0-9]+-v1\.json$/)
  const exported = JSON.parse(readFileSync(await download.path(), "utf8")) as {
    title: string
  }
  expect(exported.title).toBe(title)
})

test("shows the open comment count on the toggle and the panel", async ({
  page,
  request,
}) => {
  const { id } = await createPageDashboard(request)
  await createComment(request, id, 1, "rps", "Check the dip")
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const toggle = page.getByRole("button", {
    name: "Activity, 1 open comment",
  })
  await expect(toggle).toBeVisible()
  await expect(toggle).toHaveText("1")
  await expect(toggle).toHaveAttribute("aria-pressed", "true")
  await expect(
    page.getByRole("complementary", { name: "Activity" })
  ).toBeVisible()
  await expect(
    panel(page, rpsTitle).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
    })
  ).toBeVisible()
})

test("keeps panel editing reachable", async ({ page, request }) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const activity = page.getByRole("complementary", { name: "Activity" })
  await expect(activity).toBeVisible()
  const activityBox = await activity.boundingBox()
  expect(activityBox?.width).toBeGreaterThan(380)
  expect(activityBox?.width).toBeLessThan(420)

  await page.getByRole("button", { name: `Panel menu: ${rpsTitle}` }).click()
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click()
  const editor = page.getByRole("complementary", {
    name: `Edit ${rpsTitle}`,
  })
  await expect(editor).toBeVisible()
  await expect(editor.locator(".cm-editor")).toBeVisible()
  const editBox = await editor.boundingBox()
  expect(editBox?.width).toBeGreaterThan(700)
  expect(editBox?.width).toBeLessThan(800)

  await editor.getByRole("button", { name: "Back to activity" }).click()
  await expect(
    page.getByRole("complementary", { name: "Activity" })
  ).toBeVisible()
  const backBox = await page
    .getByRole("complementary", { name: "Activity" })
    .boundingBox()
  expect(backBox?.width).toBeGreaterThan(380)
  expect(backBox?.width).toBeLessThan(420)
})

test("keeps the composer inside the window", async ({ page, request }) => {
  const { id } = await createPageDashboard(request)
  for (let index = 1; index <= 20; index++) {
    await createComment(
      request,
      id,
      1,
      "rps",
      `Scroll comment ${index}: the card body scrolls, the composer stays put.`
    )
  }
  await page.setViewportSize({ width: 1440, height: 800 })
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const activity = page.getByRole("complementary", { name: "Activity" })
  await expect(activity.getByText(/^Scroll comment 20:/)).toBeVisible()
  await expect(activity.getByText(/^Scroll comment 1:/)).not.toBeInViewport()
  const composer = activity.getByRole("textbox", {
    name: "Comment or request",
  })
  const box = await composer.boundingBox()
  expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(800)
  const cardBox = await activity.boundingBox()
  expect((cardBox?.y ?? 0) + (cardBox?.height ?? 0)).toBeLessThanOrEqual(800)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollHeight <= window.innerHeight
    )
  ).toBe(true)
})

test("clears a posted draft after the dashboard was left", async ({
  page,
  request,
}) => {
  const { id } = await createPageDashboard(request)
  let release: () => void = () => undefined
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(`**/api/v1/dashboards/${id}/comments`, async (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    await held
    return route.fallback()
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await openDashboard(page, id)

  const composer = () =>
    page
      .getByRole("complementary", { name: "Activity" })
      .getByRole("textbox", { name: "Comment or request" })
  await composer().fill("Posted while leaving")
  await page.getByRole("button", { name: "Add comment" }).click()
  await page
    .getByRole("navigation", { name: "Breadcrumb" })
    .getByRole("link", { name: "Dashboards" })
    .click()
  await expect(page).toHaveURL("/dashboards")

  const posted = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().endsWith(`/dashboards/${id}/comments`)
  )
  release()
  expect((await posted).status()).toBe(201)

  await page.goBack()
  await expect(composer()).toBeVisible()
  await expect(composer()).toHaveValue("")
})

test("explains why Edit layout is unavailable while activity is open", async ({
  page,
  request,
}) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 1000, height: 800 })
  await openDashboard(page, id)

  await moreButton(page).click()
  const item = page.getByRole("menuitem", { name: /^Edit layout/ })
  await expect(item).toHaveAttribute("aria-disabled", "true")
  await expect(item).toHaveText("Edit layout (close activity first)")
  await page.keyboard.press("Escape")

  await page.getByRole("button", { name: "Activity", exact: true }).click()
  await moreButton(page).click()
  await expect(
    page.getByRole("menuitem", { name: "Edit layout" })
  ).not.toHaveAttribute("aria-disabled", "true")
})

test("resizes the activity card from the keyboard and remembers its width", async ({
  page,
  request,
}) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openDashboard(page, id)

  const activity = page.getByRole("complementary", { name: "Activity" })
  const grip = page.getByRole("separator", { name: "Resize panel" })
  await grip.focus()
  await page.keyboard.press("Shift+ArrowLeft")
  await expect(grip).toHaveAttribute("aria-valuenow", "464")
  expect((await activity.boundingBox())?.width).toBeCloseTo(464, 0)

  await page.reload()
  await expect(
    page.getByRole("separator", { name: "Resize panel" })
  ).toHaveAttribute("aria-valuenow", "464")

  await page.getByRole("separator", { name: "Resize panel" }).focus()
  await page.keyboard.press("Enter")
  await expect(
    page.getByRole("separator", { name: "Resize panel" })
  ).toHaveAttribute("aria-valuenow", "400")
})

test("fits a phone", async ({ page, request }) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 390, height: 844 })
  await openDashboard(page, id)

  await expect(moreButton(page)).toBeVisible()
  await expect(page.getByRole("button", { name: "Switch theme" })).toBeVisible()
  expect(await noDocumentOverflow(page)).toBe(true)
})

test("keeps a multi-value variable inside the box on a phone", async ({
  page,
  request,
}) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 390, height: 844 })
  await openDashboard(
    page,
    id,
    "?var-pod=checkout-0&var-pod=checkout-1&var-pod=checkout-2"
  )

  const input = page.getByRole("combobox", { name: "Pod" })
  await expect(input).toBeVisible()
  const control = page
    .locator('[data-slot="combobox-chips"]')
    .filter({ has: input })
    .locator("xpath=..")
  await expect(
    page
      .locator('[data-slot="combobox-chip"]')
      .filter({ hasText: "checkout-2" })
  ).toBeVisible()
  const box = await control.boundingBox()
  const chevron = await page
    .locator('[aria-label="Show Pod values"]')
    .boundingBox()
  expect((chevron?.x ?? 0) + (chevron?.width ?? 0)).toBeLessThanOrEqual(
    (box?.x ?? 0) + (box?.width ?? 0)
  )
  expect(chevron?.x).toBeGreaterThanOrEqual(box?.x ?? 0)
  expect((chevron?.y ?? 0) + (chevron?.height ?? 0)).toBeLessThanOrEqual(
    (box?.y ?? 0) + (box?.height ?? 0)
  )
  expect(await noDocumentOverflow(page)).toBe(true)
})

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test("signs out from More", async ({ page }) => {
    await signInThroughDex(page)
    const { id } = await createPageDashboard(page.request)
    await openDashboard(page, id)

    await moreButton(page).click()
    await page.getByRole("menuitem", { name: "Sign out" }).click()
    await expect(page).toHaveURL("/login")
  })
})
