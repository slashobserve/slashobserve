import type { APIRequestContext, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  startLayoutEditing,
  versionButton,
} from "./dashboards"
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
  const stream = page.waitForResponse((response) =>
    response.url().endsWith(`/dashboards/${id}/events`)
  )
  await openDashboard(page, id, "?version=1")
  await stream
  await expect(badge(page, 1)).toHaveText("v1 of 2")

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

test("does not toast the author's own save", async ({ page, request }) => {
  const { id } = await createLive(request)
  await openDashboard(page, id)
  await expect(indicator(page, "Live")).toBeVisible()

  await startLayoutEditing(page)
  await page
    .getByRole("button", { name: "Move or resize Checkout error rate" })
    .focus()
  await page.keyboard.press("ArrowRight")
  await page.getByRole("button", { name: "Save layout" }).click()
  await page
    .getByRole("dialog", { name: "Save layout" })
    .getByRole("button", { name: "Save", exact: true })
    .click()

  const toasts = page.locator("[data-sonner-toast]")
  await expect(toasts.filter({ hasText: "Saved version 2" })).toBeVisible()
  await expect(badge(page, 2)).toBeVisible()
  await page.waitForTimeout(1_500)
  await expect(toasts).toHaveCount(1)
  await expect(page.getByText("Updated to version 2")).toHaveCount(0)
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
    if (rejected < 3) {
      rejected++
      await route.fulfill({
        status: rejected === 2 ? 503 : 429,
        contentType: "application/json",
        body: JSON.stringify({ error: "busy", code: "too_many_streams" }),
      })
      return
    }
    await route.continue()
  })

  await openDashboard(page, id)
  await expect(indicator(page, "Reconnecting")).toBeVisible({
    timeout: 10_000,
  })
  await expect(indicator(page, "Live")).toBeVisible({ timeout: 30_000 })
  expect(rejected).toBe(3)
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" }).getByRole("status")
  ).toHaveText("Live updates restored")

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

test("shows not found when the stream keeps failing because the dashboard is gone", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createLive(request)
  let served = 0
  await page.route(`**/api/v1/dashboards/${id}`, async (route) => {
    if (route.request().method() !== "GET" || served++ === 0) {
      await route.continue()
      return
    }
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "not found", code: "not_found" }),
    })
  })
  await page.route("**/api/v1/dashboards/*/events", async (route) => {
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "not found", code: "not_found" }),
    })
  })

  await page.goto(`/dashboards/${id}`)
  await expect(
    page.getByRole("heading", { name: "Dashboard not found" })
  ).toBeVisible({ timeout: 20_000 })
  consoleErrors.length = 0
})
