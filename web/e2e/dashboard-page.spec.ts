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
    name: "Show activity, 1 open comment",
  })
  await expect(toggle).toBeVisible()
  await expect(toggle).toHaveText("1")
  await expect(toggle).toHaveAttribute("aria-pressed", "false")
  await expect(
    panel(page, rpsTitle).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
    })
  ).toBeVisible()
})

test("fits a phone", async ({ page, request }) => {
  const { id } = await createPageDashboard(request)
  await page.setViewportSize({ width: 390, height: 844 })
  await openDashboard(page, id)

  await expect(moreButton(page)).toBeVisible()
  await expect(page.getByRole("button", { name: "Switch theme" })).toBeVisible()
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
