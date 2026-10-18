import type { Route } from "@playwright/test"

import { expect, test, type Page } from "./fixtures"

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

function failedResourceLogged(consoleErrors: string[], status: string) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(logged[0]), 1)
}

test("shows the server's settings", async ({ page }) => {
  await openSettings(page)

  await expect(value(page, "Agent", "Provider")).toHaveText("Replay")
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
  await expect(value(page, "Sign-in", "Identity provider")).not.toHaveText("")
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
})

test("hides the identity provider line when its name is unknown", async ({
  page,
}) => {
  await mockInstance(page, { oidc_provider_name: "" })
  await openSettings(page)
  await expect(page.getByRole("button", { name: /^Account: / })).toBeVisible()
  await expect(page.getByText("Signed in with")).toHaveCount(0)
})
