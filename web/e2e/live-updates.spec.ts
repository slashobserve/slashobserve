import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard } from "./dashboards"
import { expect, test } from "./fixtures"

async function createLive(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E live")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function indicator(page: Page, label: string) {
  return page.getByRole("status").filter({ hasText: new RegExp(`^${label}$`) })
}

function badge(page: Page, version: number) {
  return page.getByText(`v${version}`, { exact: true })
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
  expect(page.url()).toBe(url)
  expect(
    await page.evaluate(
      () => (window as unknown as { liveMarker?: boolean }).liveMarker
    )
  ).toBe(true)
  await expectPanelsReady(page)
})

test("does not show the previous version's panels in the new version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createLive(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route("**/versions/2/panels/*/data*", async (route) => {
    await gate
    await route.continue()
  })

  const renamed = `${spec.title} v2`
  await addVersion(request, id, { ...spec, title: renamed }, 1)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(renamed, {
    timeout: 10_000,
  })
  await expect(badge(page, 2)).toBeVisible()

  const panels = page.locator("article[data-panel-id]")
  const count = await panels.count()
  expect(count).toBeGreaterThan(0)
  for (let index = 0; index < count; index++) {
    await expect(panels.nth(index)).toHaveAttribute("data-state", "loading")
  }

  release()
  await expectPanelsReady(page)
})

test("offers the latest when viewing an old version", async ({
  page,
  request,
}) => {
  const { id, spec } = await createLive(request)
  await addVersion(request, id, { ...spec, title: `${spec.title} v2` }, 1)
  await openDashboard(page, id, "?version=1")
  await expect(indicator(page, "Live")).toBeVisible()
  await expect(badge(page, 1)).toBeVisible()

  await addVersion(request, id, { ...spec, title: `${spec.title} v3` }, 2)

  const toast = page.locator("[data-sonner-toast]").filter({
    hasText: "Version 3 is available",
  })
  await expect(toast).toBeVisible({ timeout: 10_000 })
  await expect(badge(page, 1)).toBeVisible()
  await toast.getByRole("button", { name: "View latest" }).click()

  await expect(page).not.toHaveURL(/version=/)
  await expect(badge(page, 3)).toBeVisible()
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    `${spec.title} v3`
  )
})

test("refetches after the stream drops and reconnects", async ({
  page,
  request,
}) => {
  const { id, spec } = await createLive(request)
  let attempts = 0
  let released: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    released = resolve
  })
  await page.route("**/api/v1/dashboards/*/events", async (route) => {
    attempts++
    if (attempts === 1) {
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: `retry: 300\nevent: ready\ndata: ${JSON.stringify({ dashboard_id: id, latest_version: 1 })}\n\n`,
      })
      return
    }
    await gate
    await route.continue()
  })

  await openDashboard(page, id)
  await expect(indicator(page, "Reconnecting")).toBeVisible({
    timeout: 10_000,
  })

  const renamed = `${spec.title} v2`
  await addVersion(request, id, { ...spec, title: renamed }, 1)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  released()

  await expect(indicator(page, "Live")).toBeVisible({ timeout: 20_000 })
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(renamed, {
    timeout: 20_000,
  })
  await expect(badge(page, 2)).toBeVisible()
})

test("reconnects after the stream is rejected", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, spec } = await createLive(request)
  let rejected = 0
  await page.route("**/api/v1/dashboards/*/events", async (route) => {
    if (rejected < 2) {
      rejected++
      await route.fulfill({
        status: rejected === 1 ? 429 : 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "busy", code: "too_many_streams" }),
      })
      return
    }
    await route.continue()
  })

  await openDashboard(page, id)
  await expect(indicator(page, "Reconnecting")).toBeVisible()
  await expect(indicator(page, "Live")).toBeVisible({ timeout: 20_000 })
  expect(rejected).toBe(2)

  const renamed = `${spec.title} v2`
  await addVersion(request, id, { ...spec, title: renamed }, 1)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(renamed, {
    timeout: 10_000,
  })
  consoleErrors.length = 0
})

test("goes to sign-in when the stream is rejected after the session ended", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createLive(request)
  let signedOut = false
  await page.route("**/api/v1/auth/session", async (route) => {
    if (!signedOut) {
      await route.continue()
      return
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ authenticated: false }),
    })
  })
  await page.route("**/api/v1/dashboards/*/events", async (route) => {
    signedOut = true
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "unauthorized", code: "unauthorized" }),
    })
  })

  await openDashboard(page, id)
  await expect(page).toHaveURL(/\/login/, { timeout: 10_000 })
  consoleErrors.length = 0
})
