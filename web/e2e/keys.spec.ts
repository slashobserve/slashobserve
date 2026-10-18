import {
  request as playwrightRequest,
  type Locator,
  type Route,
} from "@playwright/test"

import {
  createServiceAccount,
  createServiceAccountKey,
  mutationHeaders,
  uniqueAccountName,
} from "./api"
import { expect, failedResourceLogged, test, type Page } from "./fixtures"

test.use({ permissions: ["clipboard-read", "clipboard-write"] })

const tokenPattern = /^sok_[A-Za-z0-9_-]{43}$/

function serverURL(): string {
  return test.info().project.use.baseURL as string
}

async function whoami(token: string) {
  const bare = await playwrightRequest.newContext({ baseURL: serverURL() })
  try {
    const response = await bare.get("/api/v1/me", {
      headers: { Authorization: `Bearer ${token}` },
    })
    const text = await response.text()
    return {
      status: response.status(),
      body: response.status() === 200 ? JSON.parse(text) : null,
    }
  } finally {
    await bare.dispose()
  }
}

function newAccountButton(page: Page) {
  return page.getByRole("button", { name: "New service account" }).first()
}

function accountSection(page: Page, name: string) {
  return page.getByRole("region", { name, exact: true })
}

function createKeyButton(page: Page, name: string) {
  return page.getByRole("button", { name: `Create key for ${name}` })
}

async function openKeysPage(page: Page) {
  await page.goto("/keys")
  await expect(
    page.getByRole("heading", { level: 1, name: "API keys" })
  ).toBeVisible()
}

async function clipboardText(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText())
}

function dateIn(days: number): string {
  const date = new Date(Date.now() + days * 86_400_000)
  const pad = (value: number) => String(value).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

async function box(locator: Locator) {
  const result = await locator.boundingBox()
  expect(result).not.toBeNull()
  return result as NonNullable<typeof result>
}

function expectSameEdge(actual: number, expected: number) {
  expect(Math.abs(actual - expected)).toBeLessThanOrEqual(1)
}

function offlineFailuresLogged(consoleErrors: string[]) {
  for (let index = consoleErrors.length - 1; index >= 0; index--) {
    if (consoleErrors[index].includes("ERR_INTERNET_DISCONNECTED")) {
      consoleErrors.splice(index, 1)
    }
  }
}

test("opens API keys from the sidebar", async ({ page }) => {
  await page.goto("/")
  const link = page
    .getByRole("navigation", { name: "Main" })
    .getByRole("link", { name: "API keys" })
  await link.click()
  await expect(page).toHaveURL(/\/keys$/)
  await expect(
    page.getByRole("heading", { level: 1, name: "API keys" })
  ).toBeVisible()
  await expect(link).toHaveAttribute("aria-current", "page")
  await expect(page).toHaveTitle("API keys - SlashObserve")
})

test("creates an account and a key and shows the key once", async ({
  page,
}) => {
  const messages: string[] = []
  page.on("console", (message) => messages.push(message.text()))
  const name = uniqueAccountName()
  await openKeysPage(page)

  await newAccountButton(page).click()
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByLabel("Name", { exact: true })).toBeFocused()
  await dialog.getByLabel("Name", { exact: true }).fill(name)
  await dialog.getByLabel("Description (optional)").fill("E2E laptop")
  await dialog.getByLabel("Description (optional)").press("Enter")

  await expect(
    dialog.getByRole("heading", { name: `Create a key for ${name}` })
  ).toBeVisible()
  await expect(dialog.getByLabel("Key name")).toBeFocused()
  await expect(dialog.getByRole("combobox", { name: "Expires" })).toContainText(
    "90 days"
  )
  await expect(dialog).toContainText(`Stops working on ${dateIn(90)}.`)
  await dialog.getByLabel("Key name").fill("laptop")
  await dialog.getByRole("button", { name: "Create key", exact: true }).click()

  await expect(
    dialog.getByRole("heading", { name: `Copy the key for ${name}` })
  ).toBeVisible()
  await expect(dialog.getByRole("button", { name: "Copy key" })).toBeFocused()
  const keyInput = dialog.getByLabel("Key", { exact: true })
  const token = await keyInput.inputValue()
  expect(token).toMatch(tokenPattern)
  expect(await page.content()).not.toContain(token)

  await dialog.getByRole("button", { name: "Copy key" }).click()
  expect(await clipboardText(page)).toBe(token)
  await expect(
    dialog.getByRole("status").filter({ hasText: "Copied" })
  ).toHaveCount(1)
  await expect(
    dialog.getByRole("region", { name: "observe CLI command" })
  ).toHaveText(`observe login --server ${serverURL()}`)

  const me = await whoami(token)
  expect(me.status).toBe(200)
  expect(me.body).toMatchObject({ kind: "service_account", name })

  await page.mouse.click(5, 5)
  await expect(dialog).toBeVisible()
  await dialog.getByRole("button", { name: "Done" }).click()
  await expect(dialog).toBeHidden()
  await expect(newAccountButton(page)).toBeFocused()

  const section = accountSection(page, name)
  await expect(
    section.getByRole("cell", { name: `${token.slice(0, 12)}…`, exact: true })
  ).toBeVisible()
  await expect(
    section.getByRole("cell", { name: "laptop", exact: true })
  ).toBeVisible()
  expect(await page.content()).not.toContain(token)

  await page.reload()
  const lastUsed = accountSection(page, name)
    .getByRole("row", { name: /laptop/ })
    .getByRole("cell")
    .nth(3)
  await expect(lastUsed).toBeVisible()
  await expect(lastUsed).not.toHaveText("Never")
  expect(await page.content()).not.toContain(token)

  expect(messages.filter((message) => message.includes("sok_"))).toEqual([])
  const stored = await page.evaluate(() =>
    [...Object.values(localStorage), ...Object.values(sessionStorage)].filter(
      (value) => value.includes("sok_")
    )
  )
  expect(stored).toEqual([])
  expect(await page.evaluate(() => indexedDB.databases())).toEqual([])
  const resources = await page.evaluate(() =>
    performance.getEntriesByType("resource").map((entry) => entry.name)
  )
  expect(resources.filter((url) => url.includes("sok_"))).toEqual([])
})

test("validates names and reports a duplicate", async ({
  page,
  request,
  consoleErrors,
}) => {
  const existing = await createServiceAccount(request, uniqueAccountName())
  const posts: string[] = []
  page.on("request", (item) => {
    const url = new URL(item.url())
    if (item.method() === "POST" && url.pathname === "/api/v1/service-accounts")
      posts.push(url.pathname)
  })
  await openKeysPage(page)
  await newAccountButton(page).click()
  const dialog = page.getByRole("dialog")
  const field = dialog.getByLabel("Name", { exact: true })
  await field.fill("Bad Name")
  await expect(
    dialog.getByText(
      "Use up to 63 lowercase letters, digits or dashes, starting with a letter or digit."
    )
  ).toBeVisible()
  await expect(field).toHaveAttribute("aria-invalid", "true")
  await field.press("Enter")
  await expect(field).toBeFocused()
  expect(posts).toEqual([])

  await field.fill(existing.name)
  await field.press("Enter")
  await expect(
    dialog.getByRole("alert").filter({
      hasText: `A service account named ${existing.name} already exists.`,
    })
  ).toBeVisible()
  await expect(field).toHaveValue(existing.name)
  expect(posts).toHaveLength(1)
  failedResourceLogged(consoleErrors, "409")
})

test("creates a key for an existing account with no expiry", async ({
  page,
  request,
}) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  await openKeysPage(page)
  await createKeyButton(page, account.name).click()

  const dialog = page.getByRole("dialog")
  await expect(
    dialog.getByRole("heading", { name: `Create a key for ${account.name}` })
  ).toBeVisible()
  await expect(dialog.getByLabel("Key name")).toBeFocused()
  await dialog.getByRole("combobox", { name: "Expires" }).click()
  await page.getByRole("option", { name: "No expiry" }).click()
  await expect(
    dialog.getByText("Never expires. Revoke it when you no longer need it.")
  ).toBeVisible()
  await dialog.getByLabel("Key name").fill("ci")
  await dialog.getByRole("button", { name: "Create key", exact: true }).click()
  await expect(
    dialog.getByRole("heading", { name: `Copy the key for ${account.name}` })
  ).toBeVisible()
  await dialog.getByRole("button", { name: "Done" }).click()
  await expect(dialog).toBeHidden()

  const row = accountSection(page, account.name).getByRole("row", {
    name: /ci/,
  })
  await expect(row.getByRole("cell").nth(4)).toHaveText("No expiry")
  await expect(createKeyButton(page, account.name)).toBeFocused()
})

test("revokes a key after confirming", async ({ page, request }) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  const key = await createServiceAccountKey(request, account.id, "ci")
  await openKeysPage(page)

  const section = accountSection(page, account.name)
  const revoke = page.getByRole("button", {
    name: `Revoke key ci for ${account.name}`,
  })
  await revoke.click()
  const confirm = page.getByRole("alertdialog", { name: "Revoke key ci?" })
  await expect(confirm).toContainText(key.display_prefix)
  await confirm.getByRole("button", { name: "Cancel" }).click()
  await expect(confirm).toBeHidden()
  await expect(revoke).toBeFocused()
  expect((await whoami(key.token)).status).toBe(200)

  await revoke.click()
  await confirm.getByRole("button", { name: "Revoke key", exact: true }).click()
  await expect(page.getByText("Revoked key ci")).toBeVisible()
  await expect(confirm).toBeHidden()
  await expect(section.getByText("No active keys.")).toBeVisible()
  await expect(createKeyButton(page, account.name)).toBeFocused()

  const toggle = section.getByRole("button", {
    name: /^(Show|Hide) .*revoked/,
  })
  await expect(toggle).toHaveText("Show 1 revoked or expired key")
  await expect(toggle).toHaveAttribute("aria-expanded", "false")
  await toggle.click()
  await expect(toggle).toHaveAttribute("aria-expanded", "true")
  await expect(
    section.getByRole("table", {
      name: `Revoked and expired keys for ${account.name}`,
    })
  ).toContainText("Revoked")
  expect((await whoami(key.token)).status).toBe(401)
})

test("shows expired keys as inactive", async ({ page, request }) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  await createServiceAccountKey(
    request,
    account.id,
    "short",
    new Date(Date.now() + 10_000).toISOString()
  )

  const section = accountSection(page, account.name)
  const toggle = section.getByRole("button", {
    name: "Show 1 revoked or expired key",
  })
  await expect(async () => {
    await openKeysPage(page)
    await expect(toggle).toBeVisible({ timeout: 3000 })
  }).toPass({ timeout: 20_000 })
  await expect(section.getByText("No active keys.")).toBeVisible()
  await toggle.click()
  const table = section.getByRole("table", {
    name: `Revoked and expired keys for ${account.name}`,
  })
  await expect(table).toContainText("Expired")
  await expect(table.getByRole("button", { name: /Revoke/ })).toHaveCount(0)
})

test("shows client snippets with this server's URLs", async ({ page }) => {
  await openKeysPage(page)
  const mcpUrl = `${serverURL()}/mcp`
  const cli = page.getByRole("region", { name: "observe CLI command" })
  await expect(cli).toHaveText(`observe login --server ${serverURL()}`)

  await page.getByRole("tab", { name: "observe CLI" }).focus()
  await page.keyboard.press("ArrowRight")
  await expect(page.getByRole("tab", { name: "Claude Code" })).toHaveAttribute(
    "aria-selected",
    "true"
  )
  const claude = JSON.parse(
    (await page
      .getByRole("region", { name: "Claude Code configuration" })
      .textContent()) ?? ""
  )
  expect(claude.mcpServers.slashobserve.url).toBe(mcpUrl)
  expect(claude.mcpServers.slashobserve.type).toBe("http")
  expect(claude.mcpServers.slashobserve.headers.Authorization).toBe(
    "Bearer ${OBSERVE_TOKEN}"
  )

  await page.keyboard.press("ArrowRight")
  await expect(page.getByRole("tab", { name: "OpenCode" })).toHaveAttribute(
    "aria-selected",
    "true"
  )
  const openCode = page.getByRole("region", { name: "OpenCode configuration" })
  const openCodeText = (await openCode.textContent()) ?? ""
  const opencode = JSON.parse(openCodeText)
  expect(opencode.mcp.slashobserve.url).toBe(mcpUrl)
  expect(opencode.mcp.slashobserve.type).toBe("remote")
  expect(opencode.mcp.slashobserve.headers.Authorization).toBe(
    "Bearer {env:OBSERVE_TOKEN}"
  )
  expect(opencode.mcp.slashobserve.oauth).toBe(false)

  await page
    .getByRole("button", { name: "Copy OpenCode configuration" })
    .click()
  expect(await clipboardText(page)).toBe(openCodeText)
  for (const text of [JSON.stringify(claude), openCodeText]) {
    expect(text).not.toContain("sok_")
  }
})

test("shows empty, loading-error and offline states", async ({
  page,
  request,
  consoleErrors,
}) => {
  const isList = (url: URL) => url.pathname === "/api/v1/service-accounts"
  const emptyList = (route: Route) =>
    route.request().method() === "GET"
      ? route.fulfill({ json: { items: [] } })
      : route.fallback()
  await page.route(isList, emptyList)
  await openKeysPage(page)
  const empty = page.locator("section", {
    has: page.getByRole("heading", { name: "No service accounts yet" }),
  })
  await expect(
    empty.getByRole("button", { name: "New service account" })
  ).toBeVisible()
  await page.unroute(isList, emptyList)

  const failList = (route: Route) =>
    route.request().method() === "GET"
      ? route.fulfill({
          status: 500,
          json: { error: "internal error", code: "internal" },
        })
      : route.fallback()
  await page.route(isList, failList)
  await page.reload()
  const alert = page.getByRole("alert").filter({
    hasText: "Could not load service accounts",
  })
  await expect(alert).toBeVisible()
  await page.unroute(isList, failList)
  await alert.getByRole("button", { name: "Retry" }).click()
  await expect(alert).toBeHidden()
  failedResourceLogged(consoleErrors, "500")

  const account = await createServiceAccount(request, uniqueAccountName())
  await page.reload()
  const isKeys = (url: URL) =>
    /^\/api\/v1\/service-accounts\/[^/]+\/keys$/.test(url.pathname)
  const offline = (route: Route) =>
    route.request().method() === "POST"
      ? route.abort("internetdisconnected")
      : route.fallback()
  await page.route(isKeys, offline)
  await createKeyButton(page, account.name).click()
  const dialog = page.getByRole("dialog")
  await dialog.getByLabel("Key name").fill("offline")
  await dialog.getByRole("button", { name: "Create key", exact: true }).click()
  await expect(
    dialog.getByRole("alert").filter({
      hasText:
        "Could not reach the server. Check your connection and try again.",
    })
  ).toBeVisible()
  await expect(dialog.getByLabel("Key name")).toHaveValue("offline")
  offlineFailuresLogged(consoleErrors)

  await page.unroute(isKeys, offline)
  await dialog.getByRole("button", { name: "Create key", exact: true }).click()
  await expect(
    dialog.getByRole("heading", { name: `Copy the key for ${account.name}` })
  ).toBeVisible()
})

test("returns focus to the header button when the empty state goes away", async ({
  page,
}) => {
  const name = uniqueAccountName()
  let created = false
  await page.route(
    (url) => url.pathname === "/api/v1/service-accounts",
    async (route) => {
      const method = route.request().method()
      if (method === "GET" && !created) {
        await route.fulfill({ json: { items: [] } })
      } else if (method === "POST") {
        const response = await route.fetch()
        created = true
        await route.fulfill({ response })
      } else {
        await route.fallback()
      }
    }
  )
  await openKeysPage(page)
  const empty = page.locator("section", {
    has: page.getByRole("heading", { name: "No service accounts yet" }),
  })
  await empty.getByRole("button", { name: "New service account" }).click()

  const dialog = page.getByRole("dialog")
  await dialog.getByLabel("Name", { exact: true }).fill(name)
  await dialog.getByLabel("Name", { exact: true }).press("Enter")
  await dialog.getByLabel("Key name").fill("first")
  await dialog.getByRole("button", { name: "Create key", exact: true }).click()
  await expect(
    dialog.getByRole("heading", { name: `Copy the key for ${name}` })
  ).toBeVisible()
  await dialog.getByRole("button", { name: "Done" }).click()
  await expect(dialog).toBeHidden()
  await expect(empty).toBeHidden()
  await expect(newAccountButton(page)).toBeFocused()
})

test("works with the keyboard only", async ({ page }) => {
  const name = uniqueAccountName()
  await openKeysPage(page)
  const trigger = newAccountButton(page)
  for (let presses = 0; presses < 20; presses++) {
    if (await trigger.evaluate((element) => element === document.activeElement))
      break
    await page.keyboard.press("Tab")
  }
  await expect(trigger).toBeFocused()
  await page.keyboard.press("Enter")

  const dialog = page.getByRole("dialog")
  await expect(dialog.getByLabel("Name", { exact: true })).toBeFocused()
  await page.keyboard.type(name)
  await page.keyboard.press("Enter")
  await expect(dialog.getByLabel("Key name")).toBeFocused()
  await page.keyboard.type("keyboard")
  await page.keyboard.press("Tab")
  const expires = dialog.getByRole("combobox", { name: "Expires" })
  await expect(expires).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("listbox")).toBeVisible()
  const thirtyDays = page.getByRole("option", { name: "30 days" })
  for (let presses = 0; presses < 4; presses++) {
    if ((await thirtyDays.getAttribute("data-highlighted")) !== null) break
    await page.keyboard.press("ArrowUp")
  }
  await expect(thirtyDays).toHaveAttribute("data-highlighted", "")
  await page.keyboard.press("Enter")
  await expect(expires).toContainText("30 days")
  await expect(expires).toBeFocused()
  await page.keyboard.press("Tab")
  await expect(
    dialog.getByRole("button", { name: "Create key", exact: true })
  ).toBeFocused()
  await page.keyboard.press("Enter")

  const copy = dialog.getByRole("button", { name: "Copy key" })
  await expect(copy).toBeFocused()
  const token = await dialog.getByLabel("Key", { exact: true }).inputValue()
  await page.keyboard.press("Enter")
  expect(await clipboardText(page)).toBe(token)
  await page.keyboard.press("Escape")
  await expect(dialog).toBeHidden()
  await expect(trigger).toBeFocused()
})

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(`on a phone in ${colorScheme} mode`, () => {
    test.use({ viewport: { width: 390, height: 844 } })

    test("fits the viewport", async ({ page, request }) => {
      await page.emulateMedia({ colorScheme })
      const longKey = "k".repeat(64)
      const tail = uniqueAccountName("w").slice(-10)
      const longName = `${"a".repeat(63 - tail.length)}${tail}`
      const keyed = await createServiceAccount(request, uniqueAccountName())
      const key = await createServiceAccountKey(request, keyed.id, longKey)
      await createServiceAccount(request, longName)
      await openKeysPage(page)

      const noOverflow = () =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      expect(await noOverflow()).toBe(true)

      await expect(page.getByRole("navigation", { name: "Main" })).toBeHidden()

      const section = accountSection(page, keyed.name)
      const revoke = section.getByRole("button", { name: /^Revoke key / })
      const revokeBox = await box(revoke)
      expect(revokeBox.x).toBeGreaterThanOrEqual(0)
      expect(revokeBox.x + revokeBox.width).toBeLessThanOrEqual(390)
      for (const header of ["Key", "Created", "Expires"]) {
        await expect(
          section.getByRole("columnheader", { name: header, exact: true })
        ).toHaveCount(0)
      }
      await expect(
        section.getByText(`${key.display_prefix}…`).first()
      ).toBeVisible()
      await expect(
        accountSection(page, longName).getByText("No active keys.")
      ).toBeVisible()

      await createKeyButton(page, longName).click()
      const dialog = page.getByRole("dialog")
      await expect(dialog.getByLabel("Key name")).toBeFocused()
      const dialogBox = await box(dialog)
      expect(dialogBox.x).toBeGreaterThanOrEqual(15.5)
      expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(374.5)
      await dialog.getByLabel("Key name").fill("phone")
      await dialog
        .getByRole("button", { name: "Create key", exact: true })
        .click()
      const input = dialog.getByLabel("Key", { exact: true })
      await expect(input).toBeVisible()
      const inputBox = await box(input)
      const copyBox = await box(
        dialog.getByRole("button", { name: "Copy key" })
      )
      const modalBox = await box(dialog)
      expect(Math.abs(inputBox.y - copyBox.y)).toBeLessThan(4)
      expect(inputBox.x).toBeGreaterThanOrEqual(modalBox.x)
      expect(copyBox.x + copyBox.width).toBeLessThanOrEqual(
        modalBox.x + modalBox.width
      )
      const command = dialog.getByRole("region", {
        name: "observe CLI command",
      })
      expect(
        await command.evaluate(
          (element) => element.scrollWidth <= element.clientWidth
        )
      ).toBe(true)
      expect(await noOverflow()).toBe(true)
      await dialog.getByRole("button", { name: "Done" }).click()
      await expect(dialog).toBeHidden()

      await page.getByRole("tab", { name: "Claude Code" }).click()
      const json = page.getByRole("region", {
        name: "Claude Code configuration",
      })
      await expect(json).toBeVisible()
      expect(
        await json.evaluate((element) => getComputedStyle(element).overflowX)
      ).toBe("auto")
      expect(await noOverflow()).toBe(true)
    })
  })
}

test("lines up at desktop width", async ({ page, request }) => {
  const names = [uniqueAccountName(), uniqueAccountName()]
  for (const name of names) {
    const account = await createServiceAccount(request, name)
    await createServiceAccountKey(request, account.id, "desk")
  }
  await openKeysPage(page)

  const headerButton = await box(newAccountButton(page))
  const firstTable = await box(page.getByRole("table").first())
  expectSameEdge(
    headerButton.x + headerButton.width,
    firstTable.x + firstTable.width
  )

  for (const name of names) {
    const button = await box(createKeyButton(page, name))
    const table = await box(
      accountSection(page, name).getByRole("table", { name: /^Active keys/ })
    )
    expectSameEdge(button.x + button.width, table.x + table.width)
  }
})

test("key tables do not scroll sideways", async ({ page, request }) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  await createServiceAccountKey(request, account.id, "active")
  const revoked = await createServiceAccountKey(request, account.id, "gone")
  const response = await request.delete(
    `/api/v1/service-accounts/${account.id}/keys/${revoked.id}`,
    { headers: mutationHeaders }
  )
  expect(response.status()).toBe(204)

  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 844 })
    await openKeysPage(page)
    const section = accountSection(page, account.name)
    await section
      .getByRole("button", { name: "Show 1 revoked or expired key" })
      .click()
    await expect(section.getByRole("table")).toHaveCount(2)
    const overflowing = await page.evaluate(
      () =>
        [...document.querySelectorAll("[data-slot=table-container]")].filter(
          (element) => element.scrollWidth > element.clientWidth
        ).length
    )
    expect(overflowing, `tables scrolling at ${width}px`).toBe(0)
  }
})
