import {
  request as playwrightRequest,
  type Locator,
  type Route,
} from "@playwright/test"

import {
  createServiceAccount,
  createServiceAccountKey,
  uniqueAccountName,
} from "./api"
import {
  expect,
  failedResourceLogged,
  tabUntilFocused,
  test,
  type Page,
} from "./fixtures"

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

function pageCreateKey(page: Page) {
  return page
    .getByRole("main")
    .getByRole("button", { name: "Create key", exact: true })
    .first()
}

function panel(page: Page) {
  return page.getByRole("complementary", { name: "Create key" })
}

function panelSubmit(page: Page) {
  return panel(page).getByRole("button", { name: "Create key", exact: true })
}

function keyRow(page: Page, keyName: string) {
  return page.locator("tr[data-key-id]").filter({ hasText: keyName })
}

async function openKeysPage(page: Page) {
  await page.goto("/keys")
  await expect(
    page.getByRole("heading", { level: 1, name: "API keys" })
  ).toBeVisible()
}

async function chooseOption(
  page: Page,
  combobox: Locator,
  option: string | RegExp
) {
  await combobox.click()
  await page.getByRole("option", { name: option, exact: true }).click()
}

async function clipboardText(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText())
}

async function box(locator: Locator) {
  const result = await locator.boundingBox()
  expect(result).not.toBeNull()
  return result as NonNullable<typeof result>
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

test("creates a key for a new service account and shows it once", async ({
  page,
}) => {
  const messages: string[] = []
  page.on("console", (message) => messages.push(message.text()))
  const accountName = uniqueAccountName()
  const keyName = uniqueAccountName("laptop")
  await openKeysPage(page)

  await pageCreateKey(page).click()
  const aside = panel(page)
  await expect(aside).toBeVisible()
  await expect(page.getByRole("dialog")).toHaveCount(0)
  const name = aside.getByLabel("Name", { exact: true })
  await expect(name).toBeFocused()
  await name.fill(keyName)
  await chooseOption(
    page,
    aside.getByRole("combobox", { name: "Service account" }),
    "New service account"
  )
  await aside.getByLabel("Service account name").fill(accountName)
  const expires = aside.getByRole("combobox", { name: "Expires" })
  await expect(expires).toContainText("In 90 days")
  await expires.click()
  await expect(page.getByRole("option")).toHaveText([
    "In 90 days",
    "In 30 days",
    "In 1 year",
    "Never",
  ])
  await expect(
    page.getByRole("option", { name: "In 90 days", exact: true })
  ).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("listbox")).toBeHidden()
  await panelSubmit(page).click()

  const tokenText = aside.locator("code")
  await expect(tokenText).toHaveText(tokenPattern)
  const token = (await tokenText.textContent()) ?? ""
  const me = await whoami(token)
  expect(me.status).toBe(200)
  expect(me.body).toMatchObject({ kind: "service_account", name: accountName })
  await expect(aside.locator("pre")).toHaveText(`export OBSERVE_TOKEN=${token}`)
  const copy = aside.getByRole("button", { name: "Copy key" })
  await expect(copy).toBeFocused()
  await copy.click()
  await expect.poll(() => clipboardText(page)).toBe(token)
  await expect(name).toBeDisabled()
  await expect(
    aside.getByRole("combobox", { name: "Service account" })
  ).toBeDisabled()
  await expect(expires).toBeDisabled()

  await aside.getByRole("button", { name: "Done" }).focus()
  await pageCreateKey(page).click()
  await expect(copy).toBeFocused()

  await page.mouse.click(5, 5)
  await expect(aside).toBeVisible()
  await aside.getByRole("button", { name: "Done" }).click()
  await expect(aside).toBeHidden()
  await expect(pageCreateKey(page)).toBeFocused()

  const row = keyRow(page, keyName)
  await expect(row).toContainText(accountName)
  await expect(row).toContainText(`${token.slice(0, 12)}…`)
  await expect(row.getByRole("cell").nth(3)).toHaveText("Never")
  await expect(row.locator('[data-status="ok"]')).toHaveText("In 90 days")
  expect(await page.content()).not.toContain(token)
  await page.reload()
  await expect(keyRow(page, keyName)).toBeVisible()
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

test("creates a key that never expires for an existing account", async ({
  page,
  request,
}) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  const keyName = uniqueAccountName("ci")
  await openKeysPage(page)
  await pageCreateKey(page).click()

  const aside = panel(page)
  await aside.getByLabel("Name", { exact: true }).fill(keyName)
  await chooseOption(
    page,
    aside.getByRole("combobox", { name: "Service account" }),
    account.name
  )
  await expect(aside.getByLabel("Service account name")).toHaveCount(0)
  await chooseOption(
    page,
    aside.getByRole("combobox", { name: "Expires" }),
    "Never"
  )
  await panelSubmit(page).click()
  await expect(aside.locator("code")).toHaveText(tokenPattern)
  await aside.getByRole("button", { name: "Done" }).click()
  await expect(aside).toBeHidden()

  const row = keyRow(page, keyName)
  await expect(row).toContainText(account.name)
  await expect(row.locator('[data-status="ok"]')).toHaveText("Never expires")
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
  await pageCreateKey(page).click()

  const aside = panel(page)
  const name = aside.getByLabel("Name", { exact: true })
  await panelSubmit(page).click()
  await expect(aside.getByText("Enter a key name.")).toBeVisible()
  await expect(name).toHaveAttribute("aria-invalid", "true")
  await expect(name).toBeFocused()

  await name.fill("duplicate check")
  await chooseOption(
    page,
    aside.getByRole("combobox", { name: "Service account" }),
    "New service account"
  )
  const accountName = aside.getByLabel("Service account name")
  await accountName.fill("Bad Name")
  await expect(
    aside.getByText(
      "Use up to 63 lowercase letters, digits or dashes, starting with a letter or digit."
    )
  ).toBeVisible()
  await expect(accountName).toHaveAttribute("aria-invalid", "true")
  await accountName.press("Enter")
  await expect(accountName).toBeFocused()
  expect(posts).toEqual([])

  await accountName.fill(existing.name)
  await accountName.press("Enter")
  await expect(
    aside.getByRole("alert").filter({
      hasText: `A service account named ${existing.name} already exists.`,
    })
  ).toBeVisible()
  await expect(accountName).toHaveValue(existing.name)
  expect(posts).toHaveLength(1)
  failedResourceLogged(consoleErrors, "409")
})

test("revokes a key after confirming", async ({ page, request }) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  const keyName = uniqueAccountName("ci")
  const key = await createServiceAccountKey(request, account.id, keyName)
  await openKeysPage(page)

  const row = keyRow(page, keyName)
  const revoke = row.getByRole("button", {
    name: `Revoke key ${keyName} for ${account.name}`,
  })
  await revoke.click()
  const confirm = page.getByRole("alertdialog", {
    name: `Revoke key ${keyName}?`,
  })
  await expect(confirm).toContainText(key.display_prefix)
  await confirm.getByRole("button", { name: "Cancel" }).click()
  await expect(confirm).toBeHidden()
  await expect(revoke).toBeFocused()
  expect((await whoami(key.token)).status).toBe(200)

  await revoke.click()
  await confirm.getByRole("button", { name: "Revoke key", exact: true }).click()
  await expect(page.getByText(`Revoked key ${keyName}`)).toBeVisible()
  await expect(confirm).toBeHidden()
  await expect(row.locator('[data-status="critical"]')).toHaveText("Revoked")
  await expect(row.getByRole("button")).toHaveCount(0)
  await expect(pageCreateKey(page)).toBeFocused()
  expect((await whoami(key.token)).status).toBe(401)
})

test("warns before a key expires", async ({ page, request }) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  const keyName = uniqueAccountName("soon")
  await createServiceAccountKey(
    request,
    account.id,
    keyName,
    new Date(Date.now() + 3 * 86_400_000).toISOString()
  )
  await openKeysPage(page)
  await expect(
    keyRow(page, keyName).locator('[data-status="warning"]')
  ).toHaveText("In 3 days")
})

test("shows client snippets with this server's URLs", async ({ page }) => {
  await openKeysPage(page)
  const mcpUrl = `${serverURL()}/mcp`
  const tabs = page.getByRole("tablist", { name: "Client" })
  await expect(tabs.getByRole("tab", { name: "Claude Code" })).toHaveAttribute(
    "aria-selected",
    "true"
  )

  const claude = page.getByRole("region", { name: "Claude Code setup" })
  const claudeText = (await claude.textContent()) ?? ""
  expect(claudeText).toContain(`--scope project slashobserve ${mcpUrl}`)
  expect(claudeText).toContain("'Authorization: Bearer ${OBSERVE_TOKEN}'")
  for (const tool of [
    "list_comments",
    "update_dashboard",
    "reply_to_comment",
    "resolve_comment",
  ]) {
    expect(claudeText).toContain(tool)
  }
  expect(claudeText).toContain("grep -qF 'For SlashObserve dashboards'")
  expect(claudeText).not.toContain('"Authorization')
  await page.getByRole("button", { name: "Copy Claude Code setup" }).click()
  await expect.poll(() => clipboardText(page)).toBe(claudeText)

  await tabs.getByRole("tab", { name: "OpenCode" }).click()
  const openCodeText =
    (await page
      .getByRole("region", { name: "OpenCode configuration" })
      .textContent()) ?? ""
  expect(openCodeText).toContain(`"url": "${mcpUrl}"`)
  expect(openCodeText).toContain("{env:OBSERVE_TOKEN}")

  await tabs.getByRole("tab", { name: "observe CLI" }).click()
  const cliText =
    (await page
      .getByRole("region", { name: "observe CLI commands" })
      .textContent()) ?? ""
  expect(cliText).toContain(`export OBSERVE_SERVER=${serverURL()}`)

  for (const text of [claudeText, openCodeText, cliText]) {
    expect(text).not.toContain("sok_")
  }
})

test("shows empty, error and offline states", async ({
  page,
  context,
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
  await expect(page.getByRole("heading", { name: "No keys yet" })).toBeVisible()
  const emptyCreate = page
    .getByRole("main")
    .getByRole("button", { name: "Create key", exact: true })
    .last()
  await emptyCreate.click()
  await expect(panel(page)).toBeVisible()
  await expect(
    panel(page).getByRole("combobox", { name: "Service account" })
  ).toContainText("New service account")
  await panel(page).getByRole("button", { name: "Cancel" }).click()
  await expect(panel(page)).toBeHidden()
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
    hasText: "Could not load API keys",
  })
  await expect(alert).toBeVisible()
  await page.unroute(isList, failList)
  await alert.getByRole("button", { name: "Retry" }).click()
  await expect(alert).toBeHidden()
  failedResourceLogged(consoleErrors, "500")

  const account = await createServiceAccount(request, uniqueAccountName())
  await page.reload()
  await pageCreateKey(page).click()
  const aside = panel(page)
  const name = aside.getByLabel("Name", { exact: true })
  await name.fill("offline")
  await chooseOption(
    page,
    aside.getByRole("combobox", { name: "Service account" }),
    account.name
  )
  await context.setOffline(true)
  await panelSubmit(page).click()
  await expect(
    aside.getByRole("alert").filter({
      hasText:
        "Could not reach the server. Check your connection and try again.",
    })
  ).toBeVisible()
  await expect(name).toHaveValue("offline")
  await expect(panelSubmit(page)).toBeEnabled()
  offlineFailuresLogged(consoleErrors)

  await context.setOffline(false)
  await panelSubmit(page).click()
  await expect(aside.locator("code")).toHaveText(tokenPattern)
})

test("keeps the loaded keys when one account's keys fail to load", async ({
  page,
  request,
  consoleErrors,
}) => {
  const healthy = await createServiceAccount(request, uniqueAccountName())
  const broken = await createServiceAccount(request, uniqueAccountName())
  const healthyKey = uniqueAccountName("healthy")
  const brokenKey = uniqueAccountName("broken")
  await createServiceAccountKey(request, healthy.id, healthyKey)
  await createServiceAccountKey(request, broken.id, brokenKey)
  const isBroken = (url: URL) =>
    url.pathname === `/api/v1/service-accounts/${broken.id}/keys`
  const fail = (route: Route) =>
    route.request().method() === "GET"
      ? route.fulfill({
          status: 500,
          json: { error: "internal error", code: "internal" },
        })
      : route.fallback()
  await page.route(isBroken, fail)
  await openKeysPage(page)

  const alert = page.getByRole("alert").filter({
    hasText: "Could not load some API keys",
  })
  await expect(alert).toContainText(broken.name)
  await expect(alert).not.toContainText(healthy.name)
  await expect(keyRow(page, healthyKey)).toBeVisible()
  await expect(keyRow(page, brokenKey)).toHaveCount(0)
  failedResourceLogged(consoleErrors, "500")

  await page.unroute(isBroken, fail)
  await alert.getByRole("button", { name: "Retry" }).click()
  await expect(alert).toBeHidden()
  await expect(keyRow(page, brokenKey)).toBeVisible()
  await expect(keyRow(page, healthyKey)).toBeVisible()
})

test("shows a dash for Created by when people fail to load", async ({
  page,
  request,
  consoleErrors,
}) => {
  const account = await createServiceAccount(request, uniqueAccountName())
  const keyName = uniqueAccountName("people")
  await createServiceAccountKey(request, account.id, keyName)
  const isUsers = (url: URL) => url.pathname === "/api/v1/users"
  const fail = (route: Route) =>
    route.fulfill({
      status: 500,
      json: { error: "internal error", code: "internal" },
    })
  await page.route(isUsers, fail)
  await openKeysPage(page)
  await expect(keyRow(page, keyName).getByRole("cell").nth(2)).toHaveText("-")
  failedResourceLogged(consoleErrors, "500")
})

test("works with the keyboard only", async ({ page }) => {
  const accountName = uniqueAccountName()
  const keyName = uniqueAccountName("keyboard")
  await openKeysPage(page)
  const trigger = pageCreateKey(page)
  await tabUntilFocused(page, trigger, 30)
  await page.keyboard.press("Enter")

  const aside = panel(page)
  const name = aside.getByLabel("Name", { exact: true })
  await expect(name).toBeFocused()
  await page.keyboard.type(keyName)
  await page.keyboard.press("Tab")
  const account = aside.getByRole("combobox", { name: "Service account" })
  await expect(account).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("listbox")).toBeVisible()
  await expect
    .poll(() =>
      page.evaluate(
        () => document.activeElement?.closest('[role="listbox"]') != null
      )
    )
    .toBe(true)
  const create = page.getByRole("option", {
    name: "New service account",
    exact: true,
  })
  await page.keyboard.press("End")
  await expect(create).toHaveAttribute("data-highlighted", "")
  await page.keyboard.press("Enter")
  await expect(account).toContainText("New service account")
  await expect(account).toBeFocused()
  await page.keyboard.press("Tab")
  const accountInput = aside.getByLabel("Service account name")
  await expect(accountInput).toBeFocused()
  await page.keyboard.type(accountName)
  await page.keyboard.press("Enter")

  const copy = aside.getByRole("button", { name: "Copy key" })
  await expect(copy).toBeFocused()
  const token = (await aside.locator("code").textContent()) ?? ""
  await page.keyboard.press("Enter")
  await expect.poll(() => clipboardText(page)).toBe(token)
  await page.keyboard.press("Escape")
  await expect(aside).toBeHidden()
  await expect(trigger).toBeFocused()
})

test("keeps a Select's Escape inside the panel", async ({ page }) => {
  await openKeysPage(page)
  await pageCreateKey(page).click()
  const aside = panel(page)
  const expires = aside.getByRole("combobox", { name: "Expires" })
  await expires.click()
  await expect(page.getByRole("listbox")).toBeVisible()
  await expect(
    page.getByRole("option", { name: "In 90 days", exact: true })
  ).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("listbox")).toBeHidden()
  await expect(aside).toBeVisible()
  await expect(expires).toBeFocused()
})

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(`on a phone in ${colorScheme} mode`, () => {
    test.use({ viewport: { width: 390, height: 844 } })

    test("fits the viewport", async ({ page, request }) => {
      await page.emulateMedia({ colorScheme })
      const longKey = "k".repeat(64)
      const account = await createServiceAccount(request, uniqueAccountName())
      await createServiceAccountKey(request, account.id, longKey)
      await openKeysPage(page)

      const noOverflow = () =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      expect(await noOverflow()).toBe(true)
      await expect(page.getByRole("navigation", { name: "Main" })).toBeHidden()

      const table = page.getByRole("region", { name: "API keys" })
      await expect(table).toBeVisible()
      expect(
        await table.evaluate(
          (element) => element.scrollWidth > element.clientWidth
        )
      ).toBe(true)
      expect(
        await table.evaluate((element) => getComputedStyle(element).overflowX)
      ).toBe("auto")

      await pageCreateKey(page).click()
      const aside = panel(page)
      await expect(aside).toBeVisible()
      const name = aside.getByLabel("Name", { exact: true })
      await expect(name).toBeFocused()
      await expect(name).toBeInViewport()
      const mainBox = await box(page.getByRole("main"))
      const panelBox = await box(aside)
      expect(panelBox.y).toBeGreaterThanOrEqual(mainBox.y + mainBox.height - 1)
      expect(panelBox.x).toBeGreaterThanOrEqual(0)
      expect(panelBox.x + panelBox.width).toBeLessThanOrEqual(390)
      expect(await noOverflow()).toBe(true)

      await name.fill("phone")
      await chooseOption(
        page,
        aside.getByRole("combobox", { name: "Service account" }),
        account.name
      )
      await panelSubmit(page).click()
      const code = aside.locator("code")
      await expect(code).toHaveText(tokenPattern)
      const codeBox = await box(code)
      const copyBox = await box(aside.getByRole("button", { name: "Copy key" }))
      const pre = await box(aside.locator("pre"))
      expect(codeBox.x).toBeGreaterThanOrEqual(panelBox.x)
      expect(copyBox.x + copyBox.width).toBeLessThanOrEqual(
        panelBox.x + panelBox.width
      )
      expect(pre.x + pre.width).toBeLessThanOrEqual(panelBox.x + panelBox.width)
      expect(await noOverflow()).toBe(true)
      await aside.getByRole("button", { name: "Done" }).click()
      await expect(aside).toBeHidden()

      const snippet = page.getByRole("region", { name: "Claude Code setup" })
      await expect(snippet).toBeVisible()
      expect(
        await snippet.evaluate(
          (element) => element.scrollWidth <= element.clientWidth
        )
      ).toBe(true)
      expect(await noOverflow()).toBe(true)
    })
  })
}
