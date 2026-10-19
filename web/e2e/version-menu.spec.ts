import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, versionButton } from "./dashboards"
import { expect, test } from "./fixtures"

const themes = ["light", "dark"] as const

async function createVersions(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E versions")
  const { id } = await createDashboard(request, spec)
  await addVersion(request, id, spec, 1, "Second save")
  await addVersion(request, id, spec, 2, "Third save")
  return { id, spec }
}

function versionMenu(page: Page) {
  return page.getByRole("menu", { name: "Versions" })
}

function historyLoaded(page: Page, id: string) {
  return page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === `/api/v1/dashboards/${id}/versions`
  )
}

for (const scheme of themes) {
  test.describe(`in the ${scheme} theme`, () => {
    test.beforeEach(async ({ page }) => {
      await page.emulateMedia({ colorScheme: scheme })
    })

    test("opens the version menu", async ({ page, request }) => {
      const { id } = await createVersions(request)
      await openDashboard(page, id)
      await expect(versionButton(page)).toHaveText("Live · v3")

      await versionButton(page).click()
      const menu = versionMenu(page)
      await expect(menu).toBeVisible()
      await expect(menu).toContainText("3 saved")
      const items = menu.getByRole("menuitemradio")
      await expect(items).toHaveCount(3)
      await expect(items.nth(0)).toContainText("v3")
      await expect(items.nth(0)).toContainText("Third save")
      await expect(items.nth(0)).toContainText("Latest")
      await expect(items.nth(0)).toHaveAttribute("aria-checked", "true")
      await expect(items.nth(1)).toContainText("v2")
      await expect(items.nth(1)).toContainText("Second save")
      await expect(items.nth(1)).toHaveAttribute("aria-checked", "false")
      await expect(items.nth(2)).toContainText("v1")
      await expect(items.nth(2)).not.toContainText("Latest")
      await expect(
        menu.getByRole("menuitem", { name: "Compare versions" })
      ).toBeVisible()

      const style = await menu.evaluate((element) => {
        const computed = getComputedStyle(element)
        return {
          boxShadow: computed.boxShadow,
          borderTopWidth: computed.borderTopWidth,
        }
      })
      expect(style.borderTopWidth).toBe("1px")
      expect(style.boxShadow).toBe("none")
    })

    test("pins and unpins from the menu", async ({ page, request }) => {
      const { id } = await createVersions(request)
      await openDashboard(page, id)
      await expect(versionButton(page)).toHaveText("Live · v3")

      await versionButton(page).click()
      await versionMenu(page)
        .getByRole("menuitemradio", { name: /^v1\b/ })
        .click()
      await expect(page).toHaveURL(/version=1/)
      await expect(versionButton(page)).toHaveText("v1 of 3")
      await expect(versionButton(page)).toHaveAccessibleName(
        "v1 of 3. Choose a version"
      )
      await expectPanelsReady(page)

      await versionButton(page).click()
      await expect(
        versionMenu(page).getByRole("menuitemradio", { name: /^v1\b/ })
      ).toHaveAttribute("aria-checked", "true")
      await versionMenu(page)
        .getByRole("menuitemradio", { name: /^v3\b/ })
        .click()
      await expect(page).not.toHaveURL(/version=/)
      await expect(versionButton(page)).toHaveText(/^(Live|Connecting) · v3$/)
      await expectPanelsReady(page)
    })

    test("compares versions", async ({ page, request }) => {
      const { id } = await createVersions(request)
      await openDashboard(page, id, "?from=now-6h")

      await versionButton(page).click()
      await versionMenu(page)
        .getByRole("menuitem", { name: "Compare versions" })
        .click()
      await expect(page).toHaveURL(
        new RegExp(`/dashboards/${id}/versions\\?back=`)
      )
      await expect(
        page.getByRole("heading", { level: 1, name: /: history$/ })
      ).toBeVisible()

      await page.getByRole("link", { name: "Back to dashboard" }).click()
      await expect(page).toHaveURL(
        new RegExp(`/dashboards/${id}\\?from=now-6h$`)
      )
      await expect(versionButton(page)).toBeVisible()
    })

    test("works from the keyboard", async ({ page, request }) => {
      const { id } = await createVersions(request)
      const loaded = historyLoaded(page, id)
      await openDashboard(page, id)
      await loaded
      await expect(versionButton(page)).toHaveText("Live · v3")

      await versionButton(page).focus()
      await page.keyboard.press("Enter")
      const items = versionMenu(page).getByRole("menuitemradio")
      await expect(items.nth(0)).toBeFocused()

      await page.keyboard.press("ArrowDown")
      await expect(items.nth(1)).toBeFocused()
      await page.keyboard.press("Enter")

      await expect(page).toHaveURL(/version=2/)
      await expect(versionButton(page)).toHaveText("v2 of 3")
      await expect(versionButton(page)).toBeFocused()
    })
  })
}
