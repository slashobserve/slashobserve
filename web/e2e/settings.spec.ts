import type { Route } from "@playwright/test"

import { expect, failedResourceLogged, test, type Page } from "./fixtures"

test.use({ permissions: ["clipboard-read", "clipboard-write"] })

function serverURL(): string {
  return test.info().project.use.baseURL as string
}

function card(page: Page, name: string) {
  return page.getByRole("region", { name, exact: true })
}

function row(page: Page, cardName: string, label: string) {
  return card(page, cardName)
    .locator('[data-slot="settings-row"]')
    .filter({ has: page.getByText(label, { exact: true }) })
}

function value(page: Page, cardName: string, label: string) {
  return row(page, cardName, label).locator(":scope > span").nth(1)
}

async function openSettings(page: Page) {
  await page.goto("/settings")
  await expect(
    page.getByRole("heading", { level: 1, name: "Settings" })
  ).toBeVisible()
}

async function mockAgent(page: Page, body: Record<string, unknown>) {
  await page.route(
    (url) => url.pathname === "/api/v1/agent",
    (route) => route.fulfill({ json: body })
  )
}

async function mockInstance(
  page: Page,
  change: Record<string, unknown>
): Promise<void> {
  await page.route(
    (url) => url.pathname === "/api/v1/instance",
    async (route) => {
      const response = await route.fetch()
      const body = (await response.json()) as Record<string, unknown>
      await route.fulfill({ response, json: { ...body, ...change } })
    }
  )
}

test("shows the server's settings", async ({ page }) => {
  await openSettings(page)

  await expect(value(page, "Agent", "Provider")).toHaveText("Replay")
  await expect(value(page, "Agent", "Model")).toHaveText("replay")
  await expect(value(page, "Agent", "Effort")).toHaveText(
    "Not used by this provider"
  )
  await expect(
    value(page, "Agent", "Status").locator('[data-status="ok"]')
  ).toHaveText("Connected")

  await expect(value(page, "This server", "Public URL")).toHaveText(serverURL())
  await expect(value(page, "This server", "MCP URL")).toHaveText(
    `${serverURL()}/mcp`
  )

  await expect(value(page, "Sign-in", "Allowed email domains")).toHaveText(
    "example.com"
  )
  await expect(value(page, "Sign-in", "Session length")).toHaveText(
    /^\d+ (days?|hours?|minutes?)$/
  )
  await expect(value(page, "Sign-in", "Identity provider")).toHaveText(
    /^https?:\/\/\S+$/
  )
})

test("keeps the row height when the server settings load", async ({ page }) => {
  let release = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(
    (url) => url.pathname === "/api/v1/instance",
    async (route) => {
      await gate
      await route.continue()
    }
  )
  await page.goto("/settings")

  const mcp = row(page, "This server", "MCP URL")
  await expect(mcp).toBeVisible()
  const loading = await mcp.boundingBox()
  expect(loading).not.toBeNull()

  release()
  await expect(page.getByRole("button", { name: "Copy MCP URL" })).toBeVisible()
  const loaded = await mcp.boundingBox()
  expect(loaded).not.toBeNull()
  expect(
    Math.abs((loaded?.height ?? 0) - (loading?.height ?? 0))
  ).toBeLessThanOrEqual(0.5)
})

test("aligns the cards with the heading and caps them at 860px", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1800, height: 900 })
  await openSettings(page)

  const heading = await page
    .getByRole("heading", { level: 1, name: "Settings" })
    .boundingBox()
  expect(heading).not.toBeNull()
  for (const name of ["Agent", "This server", "Sign-in", "Appearance"]) {
    const box = await card(page, name).boundingBox()
    expect(box).not.toBeNull()
    expect(Math.abs((box?.x ?? 0) - (heading?.x ?? 0))).toBeLessThanOrEqual(1)
    expect(box?.width).toBeLessThanOrEqual(860)
  }
})

test("copies the MCP URL", async ({ page }) => {
  await openSettings(page)
  await page.getByRole("button", { name: "Copy MCP URL" }).click()
  await expect(
    row(page, "This server", "MCP URL").getByRole("status")
  ).toHaveText("Copied")
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toBe(`${serverURL()}/mcp`)
})

test("explains a missing agent", async ({ page }) => {
  await mockAgent(page, {
    available: false,
    provider: null,
    model: null,
    effort: null,
  })
  await openSettings(page)
  await expect(value(page, "Agent", "Provider")).toHaveText("Not set")
  await expect(value(page, "Agent", "Model")).toHaveText("Not set")
  await expect(value(page, "Agent", "Effort")).toHaveText("Not set")
  await expect(
    value(page, "Agent", "Status").locator('[data-status="warning"]')
  ).toHaveText("Not configured. Set SLASHOBSERVE_LLM_PROVIDER and restart.")
})

test("shows an Anthropic agent", async ({ page }) => {
  await mockAgent(page, {
    available: true,
    provider: "anthropic",
    model: "claude-sonnet-5-5",
    effort: "medium",
  })
  await openSettings(page)
  await expect(value(page, "Agent", "Provider")).toHaveText("Anthropic")
  await expect(value(page, "Agent", "Model")).toHaveText("claude-sonnet-5-5")
  await expect(value(page, "Agent", "Effort")).toHaveText("Medium")
})

test("keeps working when the server settings fail", async ({
  page,
  consoleErrors,
}) => {
  const fail = (route: Route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: '{"error":"x","code":"internal"}',
    })
  const instance = (url: URL) => url.pathname === "/api/v1/instance"
  await page.route(instance, fail)
  await openSettings(page)

  const server = card(page, "This server")
  await expect(
    server.getByText("Could not load the server settings")
  ).toBeVisible()
  await expect(server.getByRole("button", { name: "Retry" })).toBeVisible()
  failedResourceLogged(consoleErrors, "500")

  const theme = page.getByRole("combobox", { name: "Theme" })
  await theme.click()
  await page.getByRole("option", { name: "Dark", exact: true }).click()
  await expect(page.locator("html")).toHaveClass(/\bdark\b/)

  await page.unroute(instance, fail)
  await server.getByRole("button", { name: "Retry" }).click()
  await expect(value(page, "This server", "MCP URL")).toHaveText(
    `${serverURL()}/mcp`
  )
})

test("shows the identity provider on the account button", async ({ page }) => {
  const instance = await (await page.request.get("/api/v1/instance")).json()
  const name = (instance as { oidc_provider_name: string }).oidc_provider_name
  expect(name).not.toBe("")

  await openSettings(page)
  const account = page.getByRole("button", { name: /^Account: / })
  await expect(account).toContainText(`Signed in with ${name}`)
  await expect(account).toHaveAccessibleDescription(`Signed in with ${name}`)
})

test("hides the identity provider line when its name is unknown", async ({
  page,
}) => {
  await mockInstance(page, { oidc_provider_name: "" })
  await openSettings(page)
  await expect(value(page, "This server", "MCP URL")).toHaveText(
    `${serverURL()}/mcp`
  )
  await expect(page.getByRole("button", { name: /^Account: / })).toBeVisible()
  await expect(page.getByText("Signed in with")).toHaveCount(0)
})
