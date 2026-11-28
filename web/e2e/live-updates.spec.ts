import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, versionButton } from "./dashboards"
import { expect, test } from "./fixtures"

async function createLive(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E live")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function indicator(page: Page, label: string) {
  return versionButton(page).filter({
    hasText: new RegExp(`^${label} · v\\d+$`),
  })
}

function badge(page: Page, version: number) {
  return versionButton(page).filter({
    hasText: new RegExp(`v${version}\\b`),
  })
}

test("shows a new version without reload", async ({ page, request }) => {
  const { id, spec } = await createLive(request)
  await openDashboard(page, id)
  await expect(indicator(page, "Live")).toBeVisible()
  await page.evaluate(() => {
    ;(window as unknown as { liveMarker: boolean }).liveMarker = true
  })
  const url = page.url()

  const renamed = `${spec.title} v2`
  await addVersion(
    request,
    id,
    { ...spec, title: renamed },
    1,
    "Rename from API"
  )

  await expect(page.getByRole("heading", { level: 1 })).toHaveText(renamed, {
    timeout: 10_000,
  })
  await expect(badge(page, 2)).toBeVisible()
  const toast = page.locator("[data-sonner-toast]").filter({
    hasText: "Updated to version 2",
  })
  await expect(toast).toContainText("Rename from API")
  await expect(page).toHaveURL(`${url}-v2`)
  expect(
    await page.evaluate(
      () => (window as unknown as { liveMarker?: boolean }).liveMarker
    )
  ).toBe(true)
  await expectPanelsReady(page)
})
