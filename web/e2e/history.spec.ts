import type { APIRequestContext, Locator, Page } from "@playwright/test"

import { addVersion, createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard } from "./dashboards"
import { expect, test } from "./fixtures"

const simplifiedQuery =
  'sum(rate(http_requests_total{service="checkout"}[$__rate_interval]))'

async function createHistory(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E history")
  const { id } = await createDashboard(request, spec)
  const renamed = { ...spec, title: `${spec.title} renamed` }
  await addVersion(request, id, renamed, 1, "Rename")
  const panels = renamed.panels.map((panel) =>
    panel.id === "rps" ? { ...panel, query: simplifiedQuery } : panel
  )
  const simplified = { ...renamed, panels }
  await addVersion(request, id, simplified, 2, "Simplify query")
  return { id, spec, renamed, simplified }
}

async function openHistory(page: Page, id: string, search = "") {
  await page.goto(`/dashboards/${id}/versions${search}`)
  await expect(
    page.getByRole("heading", { level: 1, name: "Version history" })
  ).toBeVisible()
  await expect(page.getByRole("table", { name: "Versions" })).toBeVisible()
}

function dialog(page: Page) {
  return page.getByRole("alertdialog")
}

function stallEvents(page: Page) {
  return page.route(/\/api\/v1\/dashboards\/[^/]+\/events$/, () => undefined)
}

function expectConflictLogged(consoleErrors: string[]) {
  const conflicts = consoleErrors.filter((message) =>
    message.includes("status of 409")
  )
  expect(conflicts).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(conflicts[0]), 1)
}

function textLeft(locator: Locator) {
  return locator.evaluate((element) => {
    const range = document.createRange()
    range.selectNodeContents(element)
    return range.getBoundingClientRect().left
  })
}

function versionRow(page: Page, version: number) {
  return page
    .getByRole("table", { name: "Versions" })
    .getByRole("row")
    .filter({
      has: page.getByRole("link", { name: `View version ${version}` }),
    })
}

test("lists versions newest first", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  const me = (await (await request.get("/api/v1/me")).json()) as {
    name: string
  }

  await openDashboard(page, id)
  await page.getByRole("link", { name: "History" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}/versions$`))
  await expect(page).toHaveTitle(
    `History - ${spec.title} renamed - SlashObserve`
  )

  const rows = page.getByRole("table", { name: "Versions" }).getByRole("row")
  await expect(rows).toHaveCount(4)
  await expect(rows.nth(1)).toContainText("v3")
  await expect(rows.nth(1)).toContainText("Latest")
  await expect(rows.nth(1)).toContainText("Simplify query")
  await expect(rows.nth(1)).toContainText(me.name)
  await expect(rows.nth(2)).toContainText("v2")
  await expect(rows.nth(2)).toContainText("Rename")
  await expect(rows.nth(2)).not.toContainText("Latest")
  await expect(rows.nth(3)).toContainText("v1")
  await expect(rows.nth(3)).toContainText(me.name)
  await expect(
    page.getByRole("button", { name: "Restore version 3" })
  ).toHaveCount(0)

  await page.getByRole("link", { name: "View version 2" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}\\?version=2$`))
  await openHistory(page, id)
  await page.getByRole("link", { name: "View version 3" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
})

test("shows the Spec diff between two versions", async ({ page, request }) => {
  const { id, spec, renamed } = await createHistory(request)
  await openHistory(page, id)

  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v2 to v3" })
  ).toBeVisible()
  const panelGroup = page.getByRole("region", {
    name: 'Panel "Checkout requests per second"',
  })
  await expect(panelGroup.getByText("Changed")).toBeVisible()
  await expect(panelGroup.getByText("query", { exact: true })).toBeVisible()
  await expect(panelGroup.getByRole("group", { name: "Before" })).toContainText(
    "sum by (pod)"
  )
  await expect(panelGroup.getByRole("group", { name: "After" })).toContainText(
    simplifiedQuery
  )

  await page.getByRole("combobox", { name: "From" }).click()
  await page.getByRole("option", { name: "v1", exact: true }).click()
  await expect(page).toHaveURL(/from=1/)
  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v1 to v3" })
  ).toBeVisible()
  await expect(
    page.getByRole("heading", { level: 2, name: "Changes from v1 to v3" })
  ).not.toBeFocused()
  const settings = page.getByRole("region", { name: "Dashboard settings" })
  await expect(settings.getByText("Changed")).toBeVisible()
  await expect(settings.getByText("title", { exact: true })).toBeVisible()
  await expect(settings.getByRole("group", { name: "Before" })).toHaveText(
    new RegExp(`${spec.title}$`)
  )
  await expect(settings.getByRole("group", { name: "After" })).toHaveText(
    new RegExp(`${renamed.title}$`)
  )
})

test("compares a version with its parent", async ({ page, request }) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)
  await page.evaluate(() => {
    const focused: string[] = []
    Object.assign(window, { focusedHeadings: focused })
    document.addEventListener("focusin", (event) => {
      if (event.target instanceof HTMLHeadingElement) {
        focused.push(event.target.textContent ?? "")
      }
    })
  })

  await page
    .getByRole("button", { name: "Compare version 2 with version 1" })
    .click()
  await expect(page).toHaveURL(/from=1&to=2/)
  const heading = page.getByRole("heading", {
    level: 2,
    name: "Changes from v1 to v2",
  })
  await expect(heading).toBeFocused()
  expect(
    await page.evaluate(
      () => (window as unknown as { focusedHeadings: string[] }).focusedHeadings
    )
  ).toEqual(["Changes from v1 to v2"])
  await expect(
    page.getByRole("region", { name: "Dashboard settings" })
  ).toBeVisible()
  await expect(page.getByRole("region", { name: /^Panel / })).toHaveCount(0)
})

test("restores v1 from history", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await expect(dialog(page)).toBeVisible()
  await expect(
    dialog(page).getByRole("heading", { name: "Restore version 1?" })
  ).toBeVisible()
  await expect(dialog(page)).toContainText(
    "This saves a new version 4 with the dashboard exactly as it was in version 1. Nothing is deleted."
  )
  await expect(
    dialog(page).getByRole("button", { name: "Cancel" })
  ).toBeFocused()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()

  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expect(
    page.getByRole("main").getByText("v4", { exact: true })
  ).toBeVisible()

  await page.getByRole("link", { name: "History" }).click()
  const row = page
    .getByRole("table", { name: "Versions" })
    .getByRole("row")
    .nth(1)
  await expect(row).toContainText("v4")
  await expect(row).toContainText("Restored from v1")
  await expect(row).toContainText("Restore version 1")
})

test("cancelling the restore dialog changes nothing", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 2" }).click()
  await dialog(page).getByRole("button", { name: "Cancel" }).click()
  await expect(dialog(page)).toHaveCount(0)
  await expect(
    page.getByRole("button", { name: "Restore version 2" })
  ).toBeFocused()
  await expect(
    page.getByRole("table", { name: "Versions" }).getByRole("row")
  ).toHaveCount(4)
})

test("restores from the version banner", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openDashboard(
    page,
    id,
    "?version=1&from=now-6h&refresh=30s&var-pod=checkout-1"
  )

  await page.getByRole("button", { name: "Restore this version" }).click()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()

  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).not.toHaveURL(/version=/)
  await expect(page).toHaveURL(/from=now-6h/)
  await expect(page).toHaveURL(/refresh=30s/)
  await expect(page).toHaveURL(/var-pod=checkout-1/)
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
  await expect(
    page.getByRole("main").getByText("v4", { exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Restore this version" })
  ).toHaveCount(0)
})

test("handles a conflict during restore", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, simplified } = await createHistory(request)
  await stallEvents(page)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await expect(dialog(page)).toBeVisible()
  await addVersion(
    request,
    id,
    { ...simplified, title: uniqueTitle("E2E history other") },
    3,
    "Concurrent change"
  )

  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(dialog(page)).toContainText("Version 4 was saved meanwhile.")
  await expect(dialog(page)).toContainText("This saves a new version 5")
  expectConflictLogged(consoleErrors)

  await dialog(page)
    .getByRole("button", { name: "Restore on top of version 4" })
    .click()
  await expect(page.getByText("Restored version 1 as version 5")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}$`))
})

test("a save that lands while the dialog is open still gets the conflict flow", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id, simplified } = await createHistory(request)
  await openHistory(page, id)

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await expect(dialog(page)).toContainText("This saves a new version 4")
  await addVersion(
    request,
    id,
    { ...simplified, title: uniqueTitle("E2E history other") },
    3,
    "Concurrent change"
  )
  const saved = page.locator("tr", { hasText: "Concurrent change" })
  await expect(saved).toContainText("Latest")
  await expect(dialog(page)).toContainText("This saves a new version 4")

  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(dialog(page)).toContainText("Version 4 was saved meanwhile.")
  expectConflictLogged(consoleErrors)
})

test("keeps focus inside the dialog while a restore is pending", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)
  let release: () => void = () => undefined
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(/\/versions\/1\/restore$/, async (route) => {
    await gate
    await route.continue()
  })

  await page.getByRole("button", { name: "Restore version 1" }).click()
  const confirm = dialog(page).getByRole("button", {
    name: "Restore",
    exact: true,
  })
  await confirm.click()
  await expect(confirm).toHaveAttribute("aria-busy", "true")
  await expect(confirm).toBeFocused()
  await page.keyboard.press("Shift+Tab")
  await expect(
    dialog(page).getByRole("button", { name: "Cancel" })
  ).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(dialog(page)).toBeVisible()
  release()
  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
})

test("shows a version saved elsewhere without reloading", async ({
  page,
  request,
}) => {
  const { id, simplified } = await createHistory(request)
  await openHistory(page, id)
  await expect(versionRow(page, 3)).toContainText("Latest")
  await expect(
    page.getByRole("button", { name: "Restore version 3" })
  ).toHaveCount(0)

  await addVersion(
    request,
    id,
    { ...simplified, title: uniqueTitle("E2E history other") },
    3,
    "Saved elsewhere"
  )

  await expect(versionRow(page, 4)).toContainText("Saved elsewhere")
  await expect(versionRow(page, 4)).toContainText("Latest")
  await expect(versionRow(page, 3)).not.toContainText("Latest")
  await expect(
    page.getByRole("button", { name: "Restore version 3" })
  ).toBeVisible()
  expect(await page.locator("[data-sonner-toast]").count()).toBe(0)
})

test("explains a transient restore failure as try again later", async ({
  page,
  request,
  consoleErrors,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)
  await page.route(/\/versions\/1\/restore$/, (route) =>
    route.fulfill({
      status: 422,
      contentType: "application/json",
      body: JSON.stringify({
        error: "spec is invalid",
        code: "invalid_spec",
        issues: [
          {
            path: "/panels/0/query",
            code: "datasource_unavailable",
            message: "prometheus returned 502",
          },
          {
            path: "/panels/1/query",
            code: "datasource_unavailable",
            message: "prometheus returned 502",
          },
          {
            path: "/panels/2/query",
            code: "query_timeout",
            message: "query timed out",
          },
        ],
      }),
    })
  )

  await page.getByRole("button", { name: "Restore version 1" }).click()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(dialog(page)).toContainText(
    "This version could not be restored right now."
  )
  await expect(dialog(page)).not.toContainText("cannot be restored")
  await expect(dialog(page).getByRole("listitem")).toHaveText([
    "This version could not be checked right now. Try again later.",
  ])
  const failed = consoleErrors.filter((message) =>
    message.includes("status of 422")
  )
  expect(failed).toHaveLength(1)
  consoleErrors.splice(consoleErrors.indexOf(failed[0]), 1)
})

test("keeps the dashboard view through history and back", async ({
  page,
  request,
}) => {
  const { id, spec } = await createHistory(request)
  await openDashboard(page, id, "?from=now-6h&refresh=30s&var-pod=checkout-1")

  await page.getByRole("link", { name: "History" }).click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}/versions\\?back=`))
  await page.getByRole("link", { name: "Back to dashboard" }).click()
  await expect(page).toHaveURL(
    new RegExp(
      `/dashboards/${id}\\?from=now-6h&refresh=30s&var-pod=checkout-1$`
    )
  )

  await page.getByRole("link", { name: "History" }).click()
  await page.getByRole("button", { name: "Restore version 1" }).click()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).toHaveURL(
    new RegExp(
      `/dashboards/${id}\\?from=now-6h&refresh=30s&var-pod=checkout-1$`
    )
  )
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(spec.title)
})

test("the history link keeps a pinned version and drops it after a restore", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openDashboard(page, id, "?version=2&from=now-1h")

  await page.getByRole("link", { name: "History" }).click()
  await page.getByRole("link", { name: "Back to dashboard" }).click()
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${id}\\?from=now-1h&version=2$`)
  )

  const back = encodeURIComponent("from=now-1h&version=2&panel=rps&tab=query")
  await openHistory(page, id, `?back=${back}`)
  await page.getByRole("link", { name: "Back to dashboard" }).click()
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${id}\\?from=now-1h&version=2&panel=rps&tab=query$`)
  )

  await openHistory(page, id, `?back=${back}`)
  await page.getByRole("button", { name: "Restore version 1" }).click()
  await dialog(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click()
  await expect(page.getByText("Restored version 1 as version 4")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${id}\\?from=now-1h$`))
})

test("the View links keep the dashboard search", async ({ page, request }) => {
  const { id } = await createHistory(request)
  const back = encodeURIComponent(
    "from=now-6h&var-pod=checkout-1&version=2&panel=rps&tab=query"
  )
  await openHistory(page, id, `?back=${back}`)

  await page.getByRole("link", { name: "View version 1" }).click()
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${id}\\?from=now-6h&var-pod=checkout-1&version=1$`)
  )

  await openHistory(page, id, `?back=${back}`)
  await page.getByRole("link", { name: "View version 3" }).click()
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${id}\\?from=now-6h&var-pod=checkout-1$`)
  )
})

test("accepts only dashboard search keys in the back parameter", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  const back = encodeURIComponent(
    "from=now-6h&redirect=https://evil.example&version=zz&var-pod=checkout-1"
  )
  await openHistory(page, id, `?back=${back}`)

  await page.getByRole("link", { name: "Back to dashboard" }).click()
  await expect(page).toHaveURL(
    new RegExp(`/dashboards/${id}\\?from=now-6h&var-pod=checkout-1$`)
  )
})

test("keeps the version actions in fixed columns", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)

  const positions = async (name: (version: number) => string) =>
    Promise.all(
      [3, 2, 1].map(async (version) => {
        const box = await versionRow(page, version)
          .getByRole(name(version).startsWith("View") ? "link" : "button", {
            name: name(version),
          })
          .boundingBox()
        return box?.x
      })
    )
  const views = await positions((version) => `View version ${version}`)
  expect(new Set(views).size).toBe(1)
  const compares = await positions((version) =>
    version === 1
      ? "Compare version 1 with an empty dashboard"
      : `Compare version ${version} with version ${version - 1}`
  )
  expect(new Set(compares).size).toBe(1)

  const header = page.getByRole("columnheader", { name: "Actions" })
  for (const version of [3, 2, 1]) {
    const view = versionRow(page, version).getByRole("link", {
      name: `View version ${version}`,
    })
    expect(await textLeft(view)).toBeCloseTo(await textLeft(header), 0)
  }
})

test("resets Show all when the compared versions change", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id, "?from=0&to=2")
  await expect(
    page.getByRole("heading", {
      level: 2,
      name: "Changes from an empty dashboard to v2",
    })
  ).toBeVisible()
  const after = page
    .getByRole("region", { name: "Dashboard settings" })
    .getByRole("group", { name: "After" })
  await expect(after.getByRole("button", { name: "Show all" })).toBeVisible()

  await page.getByRole("combobox", { name: "To" }).click()
  await page.getByRole("option", { name: "v1", exact: true }).click()
  await expect(page).toHaveURL(/from=0&to=1/)
  await after.getByRole("button", { name: "Show all" }).click()
  await expect(after.getByRole("button", { name: "Show all" })).toHaveCount(0)

  await page.getByRole("combobox", { name: "To" }).click()
  await page.getByRole("option", { name: "v2", exact: true }).click()
  await expect(page).toHaveURL(/from=0&to=2/)
  await expect(after.getByRole("button", { name: "Show all" })).toBeVisible()
})

test("sizes the version menu to its trigger and aligns the text", async ({
  page,
  request,
}) => {
  const { id } = await createHistory(request)
  await openHistory(page, id)

  const trigger = page.getByRole("combobox", { name: "From" })
  await trigger.click()
  const popup = page.locator('[data-slot="select-content"]')
  await expect(popup).toBeVisible()
  const option = page.getByRole("option", { name: "v2", exact: true })
  const optionText = option.locator("> *").first()
  const triggerText = trigger.locator('[data-slot="select-value"]')
  await expect
    .poll(async () => (await popup.boundingBox())?.width)
    .toBeCloseTo((await trigger.boundingBox())?.width ?? 0, 0)
  await expect
    .poll(async () => textLeft(optionText))
    .toBeCloseTo(await textLeft(triggerText), 0)
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("lines the History link up with the Live status", async ({
    page,
    request,
  }) => {
    const { id } = await createHistory(request)
    await openDashboard(page, id, "?from=now-6h")

    const live = await page.getByRole("status").boundingBox()
    const history = await page
      .getByRole("link", { name: "History" })
      .boundingBox()
    const centerY = (box: typeof live) => (box?.y ?? 0) + (box?.height ?? 0) / 2
    expect(centerY(history)).toBeCloseTo(centerY(live), 0)
    expect((history?.x ?? 0) + (history?.width ?? 0)).toBeCloseTo(374, 0)
    expect(live?.x).toBeCloseTo(16, 0)
  })

  test("aligns the View link with the row text", async ({ page, request }) => {
    const { id } = await createHistory(request)
    await openHistory(page, id)

    const row = versionRow(page, 2)
    const label = row.getByText("v2", { exact: true })
    const view = row.getByRole("link", { name: "View version 2" })
    expect(await textLeft(view)).toBeCloseTo(await textLeft(label), 0)
  })
})

test("diffs v1 against an empty dashboard", async ({ page, request }) => {
  const { id, spec } = await createHistory(request)
  await openHistory(page, id)

  await page
    .getByRole("button", { name: "Compare version 1 with an empty dashboard" })
    .click()
  await expect(page).toHaveURL(/from=0&to=1/)
  await expect(
    page.getByRole("heading", {
      level: 2,
      name: "Changes from an empty dashboard to v1",
    })
  ).toBeFocused()
  const settings = page.getByRole("region", { name: "Dashboard settings" })
  const changes = settings.getByRole("listitem")
  await expect(changes).toHaveCount(1)
  await expect(changes.first()).toContainText("whole dashboard")
  await expect(changes.first().getByText(/^(Added|Changed)$/)).toBeVisible()
  const after = changes.first().getByRole("group", { name: "After" })
  await after.getByRole("button", { name: "Show all" }).click()
  await expect(after).toContainText(spec.title)
  await expect(after.getByRole("button", { name: "Show all" })).toHaveCount(0)
})
