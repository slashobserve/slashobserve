import {
  expect,
  type Locator,
  type Page,
  type Response,
} from "@playwright/test"

export async function openDashboard(page: Page, id: string, search = "") {
  await page.goto(`/d/${id}${search}`)
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
}

export function versionButton(page: Page) {
  return page
    .getByRole("navigation", { name: "Breadcrumb" })
    .getByRole("button", { name: /\. Choose a version$/ })
}

export async function expectNoAnimations(locator: Locator) {
  await expect
    .poll(() =>
      locator.evaluateAll(
        (elements) =>
          elements.flatMap((element) => element.getAnimations()).length
      )
    )
    .toBe(0)
}

export async function selectPanel(page: Page, title: string) {
  await panel(page, title)
    .getByRole("button", { name: title, exact: true })
    .click()
  await expect(panel(page, title)).toHaveAttribute("data-selected", "")
  await expect(panel(page, title)).toHaveAttribute("aria-current", "true")
  await expectNoAnimations(page.locator(".react-grid-item"))
}

export async function selectPanelToArrange(page: Page, title: string) {
  await selectPanel(page, title)
  await expect(
    page.getByRole("button", { name: `Move or resize ${title}` })
  ).toBeVisible()
}

export function unsavedLayout(page: Page) {
  return page.getByRole("region", { name: "Unsaved layout" })
}

export async function openVersionsFromMenu(page: Page) {
  await versionButton(page).click()
  await page.getByRole("menuitem", { name: "Compare versions" }).click()
}

export function panel(page: Page, title: string) {
  return page.getByRole("article", { name: title })
}

export async function expectPanelsReady(page: Page) {
  const panels = page.locator("article[data-panel-id]")
  await panels.first().waitFor({ state: "attached" })
  const count = await panels.count()
  expect(count).toBeGreaterThan(0)
  for (let index = 0; index < count; index++) {
    const item = panels.nth(index)
    await item.scrollIntoViewIfNeeded()
    await expect(item).toHaveAttribute("data-state", "ready", {
      timeout: 30_000,
    })
  }
  await expect(page.getByText("No data")).toHaveCount(0)
}

export function panelDataResponse(
  page: Page,
  panelId: string,
  predicate?: (url: URL) => boolean
): Promise<Response> {
  return page.waitForResponse(
    (response) =>
      response.url().includes(`/panels/${panelId}/data`) &&
      response.ok() &&
      (predicate?.(new URL(response.url())) ?? true)
  )
}
