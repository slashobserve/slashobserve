import type { APIRequestContext } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  panelDataResponse,
} from "./dashboards"
import { expect, test } from "./fixtures"

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E overview")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

test("renders every panel with data", async ({ page, request }) => {
  const { id, spec } = await createOverview(request)
  await openDashboard(page, id)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expectPanelsReady(page)

  const logs = panel(page, "Log rows by service")
  await expect(
    logs.getByRole("columnheader", { name: "service" })
  ).toBeVisible()
  expect(await logs.getByRole("row").count()).toBeGreaterThan(1)
  await expect(page).toHaveTitle(`${spec.title} - SlashObserve`)
})

test("changing the time range updates the URL and reloads panels", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  await page.getByRole("button", { name: /^Time range/ }).click()
  const reloaded = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.get("from") === "now-1h"
  )
  await page.getByRole("button", { name: "Last 1 hour", exact: true }).click()
  await reloaded
  await expect(page).toHaveURL(/from=now-1h&to=now/)

  await page.reload()
  await expect(
    page.getByRole("button", { name: "Time range: Last 1 hour" })
  ).toBeVisible()
  await expectPanelsReady(page)
})

test("a variable filters the series and survives reload", async ({
  page,
  request,
}) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const service = page.getByRole("combobox", { name: "Service" })
  await service.click()
  const filtered = panelDataResponse(
    page,
    "rps",
    (url) => url.searchParams.getAll("var-service").join(",") === "checkout"
  )
  await page.getByRole("option", { name: "checkout" }).click()
  await expect(page).toHaveURL(/var-service=checkout(&|$)/)
  const body = (await (await filtered).json()) as {
    frames: { labels?: { service_name?: string } }[]
  }
  expect(body.frames.length).toBeGreaterThan(0)
  for (const frame of body.frames)
    expect(frame.labels?.service_name).toBe("checkout")
  await page.keyboard.press("Escape")

  await page.reload()
  const chips = page.locator('[data-slot="combobox-chips"]')
  await expect(chips).toContainText("checkout")
  await expectPanelsReady(page)

  await page.getByRole("combobox", { name: "Service" }).click()
  await page.getByRole("option", { name: "All", exact: true }).click()
  await expect(page).toHaveURL(/var-service=%24__all/)
})

test("is usable from the keyboard", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)

  const trigger = page.getByRole("button", { name: /^Time range/ })
  await trigger.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("textbox", { name: "From" })).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("textbox", { name: "From" })).toHaveCount(0)
  await expect(trigger).toBeFocused()

  const visited: string[] = []
  for (let step = 0; step < 80 && !visited.includes("logs"); step++) {
    await page.keyboard.press("Tab")
    const menu = await page.evaluate(
      () => document.activeElement?.getAttribute("data-panel-menu") ?? null
    )
    if (menu) visited.push(menu)
  }
  expect(visited).toContain("rps")
  expect(visited).toContain("logs")
  expect(visited.indexOf("rps")).toBeLessThan(visited.indexOf("logs"))
})

test("never exposes datasource connection details", async ({
  page,
  request,
}) => {
  const bodies: string[] = []
  const reads: Promise<void>[] = []
  page.on("response", (response) => {
    const type = response.headers()["content-type"] ?? ""
    if (response.url().includes("/api/") && type.includes("json")) {
      reads.push(
        response.text().then(
          (text) => void bodies.push(text),
          () => undefined
        )
      )
    }
  })
  const { id } = await createOverview(request)
  await openDashboard(page, id)
  await expectPanelsReady(page)
  await Promise.all(reads)

  expect(bodies.length).toBeGreaterThan(0)
  for (const body of bodies) {
    for (const secret of [
      "prometheus:9090",
      "clickhouse:9000",
      "clickhouse:8123",
      "slashobserve_ro",
    ]) {
      expect(body).not.toContain(secret)
    }
  }
})
