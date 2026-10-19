import { createDashboard, loadSpec, uniqueTitle } from "./api"
import type { Route } from "@playwright/test"

import { expect, test, type Page } from "./fixtures"

const listPath = "/api/v1/dashboards"
const newTabSuffix = " (opens in a new tab)"

const cardColors = {
  light: { background: "rgb(255, 255, 255)", border: "rgb(228, 228, 231)" },
  dark: { background: "rgb(17, 17, 19)", border: "rgb(42, 42, 46)" },
} as const

function isListRequest(url: URL): boolean {
  return url.pathname === listPath
}

type Summary = ReturnType<typeof fakeSummary>

function fakeSummary(
  title: string,
  overrides: { description?: string | null; datasource_uids?: string[] } = {}
) {
  const now = new Date().toISOString()
  const author = {
    kind: "user" as const,
    id: crypto.randomUUID(),
    name: "Fake Author",
  }
  return {
    id: crypto.randomUUID(),
    title,
    description: null as string | null,
    tags: [] as string[],
    latest_version: 3,
    created_at: now,
    updated_at: now,
    updated_by: author,
    creator: {
      author,
      origin: "web" as const,
      origin_client: null,
      via: "direct" as const,
    },
    datasource_uids: ["prom"],
    open_comment_count: 0,
    ...overrides,
  }
}

function listBody(dashboards: Summary[], nextCursor: string | null = null) {
  return {
    dashboards,
    next_cursor: nextCursor,
    total_count: dashboards.length,
  }
}

function dashboardsCard(page: Page) {
  return page.getByRole("region", { name: "Dashboards" })
}

function filterBox(page: Page) {
  return page.getByRole("textbox", { name: "Filter dashboards" })
}

function rowFor(page: Page, title: string) {
  return page
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: title }) })
}

async function filterByTitle(page: Page, title: string) {
  await filterBox(page).fill(title)
  await expect(rowFor(page, title)).toBeVisible()
  await expect(dashboardsCard(page).getByRole("row")).toHaveCount(2)
}

function searchParam(page: Page, name: string) {
  return new URL(page.url()).searchParams.get(name)
}

test("lists dashboards newest first", async ({ page, request }) => {
  const titleA = uniqueTitle("List A")
  const titleB = uniqueTitle("List B")
  await createDashboard(request, { ...loadSpec("overview"), title: titleA })
  await createDashboard(request, { ...loadSpec("overview"), title: titleB })

  await page.goto("/dashboards")
  const linkA = page.getByRole("link", { name: titleA })
  const linkB = page.getByRole("link", { name: titleB })
  await expect(linkA).toBeVisible()
  await expect(linkB).toBeVisible()

  const boxA = await linkA.boundingBox()
  const boxB = await linkB.boundingBox()
  expect(boxB!.y).toBeLessThan(boxA!.y)

  for (const title of [titleA, titleB]) {
    await expect(
      rowFor(page, title).getByText("v1", { exact: true })
    ).toBeVisible()
  }
  await expect(dashboardsCard(page)).toBeVisible()
  await expect(page.getByRole("columnheader")).toHaveText([
    "Title",
    "Made by",
    "Datasources",
    "Open comments",
    "Version",
    "Updated",
  ])
})

for (const scheme of ["light", "dark"] as const) {
  test.describe(scheme, () => {
    test.use({ colorScheme: scheme })

    test("lists a dashboard with its maker and datasources", async ({
      page,
      request,
    }) => {
      const title = uniqueTitle("List")
      await createDashboard(request, { ...loadSpec("overview"), title })
      const listed = await request.get(
        `${listPath}?q=${encodeURIComponent(title)}`
      )
      const body = (await listed.json()) as {
        dashboards: { creator: { author: { name: string } } }[]
      }
      expect(body.dashboards).toHaveLength(1)
      const maker = body.dashboards[0].creator.author.name

      await page.goto("/dashboards")
      await filterByTitle(page, title)
      const cells = rowFor(page, title).getByRole("cell")
      await expect(cells.nth(1)).toHaveText(maker)
      await expect(cells.nth(2)).toContainText("prom")
      await expect(cells.nth(3)).toHaveText("None")
      await expect(cells.nth(4)).toHaveText("v1")

      const card = dashboardsCard(page)
      await expect(card).toHaveCSS(
        "background-color",
        cardColors[scheme].background
      )
      await expect(card).toHaveCSS(
        "border-top-color",
        cardColors[scheme].border
      )
    })
  })
}

test("opens a dashboard in a new tab", async ({ page, context, request }) => {
  const title = uniqueTitle("List tab")
  const created = await createDashboard(request, {
    ...loadSpec("overview"),
    title,
  })
  await page.goto("/dashboards")
  await filterByTitle(page, title)

  const link = page.getByRole("link", { name: title })
  await expect(link).toHaveAttribute("target", "_blank")
  await expect(link).toHaveAccessibleName(`${title}${newTabSuffix}`)
  const [popup] = await Promise.all([
    context.waitForEvent("page"),
    link.click(),
  ])
  await expect(popup).toHaveURL(new RegExp(`/dashboards/${created.id}`))
  await expect(popup.getByRole("heading", { level: 1 })).toHaveText(title)
  await expect(page).toHaveURL(/\/dashboards(\?.*)?$/)
  await popup.close()
})

test("filters by title, maker and datasource, in the URL", async ({
  page,
  request,
}) => {
  const title = uniqueTitle("List filter")
  await createDashboard(request, { ...loadSpec("overview"), title })
  await page.goto("/dashboards")

  await filterByTitle(page, title)
  await expect.poll(() => searchParam(page, "q")).toBe(title)

  await page.getByRole("button", { name: /^Made by:/ }).click()
  await page.getByRole("menuitemradio", { name: "Me" }).click()
  await expect(page.getByRole("button", { name: "Made by: me" })).toBeVisible()
  await expect.poll(() => searchParam(page, "made_by")).toBe("me")
  await expect(rowFor(page, title)).toBeVisible()

  await page.getByRole("button", { name: /^Datasource:/ }).click()
  await page
    .getByRole("menuitemradio", { name: "prom (prometheus)", exact: true })
    .click()
  await expect(
    page.getByRole("button", { name: "Datasource: prom" })
  ).toBeVisible()
  await expect.poll(() => searchParam(page, "datasource")).toBe("prom")
  await expect(rowFor(page, title)).toBeVisible()

  await page.reload()
  await expect(filterBox(page)).toHaveValue(title)
  await expect(page.getByRole("button", { name: "Made by: me" })).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Datasource: prom" })
  ).toBeVisible()
  await expect(rowFor(page, title)).toBeVisible()

  await filterBox(page).fill(`${title} no such dashboard`)
  await expect(
    page.getByRole("heading", { name: "No dashboards match these filters." })
  ).toBeVisible()
  await expect(dashboardsCard(page)).toHaveCount(0)
  await page.getByRole("button", { name: "Clear filters" }).click()
  await expect.poll(() => new URL(page.url()).search).toBe("")
  await expect(filterBox(page)).toHaveValue("")
  await expect(
    page.getByRole("button", { name: "Made by: anyone" })
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Datasource: any" })
  ).toBeVisible()
  await expect(rowFor(page, title)).toBeVisible()
})

test("ignores filter values in the URL that the server would refuse", async ({
  page,
}) => {
  const listed: URL[] = []
  page.on("request", (request) => {
    const url = new URL(request.url())
    if (isListRequest(url)) listed.push(url)
  })
  await page.goto("/dashboards?made_by=mcp:%20x&datasource=_bad")
  await expect(
    page.getByRole("button", { name: "Made by: anyone" })
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Datasource: any" })
  ).toBeVisible()
  await expect(page.getByRole("alert")).toHaveCount(0)
  expect(listed.length).toBeGreaterThan(0)
  for (const url of listed) {
    expect(url.searchParams.has("made_by")).toBe(false)
    expect(url.searchParams.has("datasource")).toBe(false)
  }
})

test("counts matches", async ({ page, request }) => {
  const title = uniqueTitle("List count")
  await createDashboard(request, { ...loadSpec("overview"), title })
  await page.goto("/dashboards")

  await filterByTitle(page, title)
  await expect(page.getByText("1 dashboard", { exact: true })).toBeVisible()

  await filterBox(page).fill(`${title} no such dashboard`)
  await expect(page.getByText("0 dashboards", { exact: true })).toBeVisible()
})

function filteredList(
  row: Summary,
  options: { gate?: Promise<void>; seen?: (q: string | null) => void } = {}
) {
  return async (route: Route) => {
    const params = new URL(route.request().url()).searchParams
    const filtered = ["q", "made_by", "datasource"].some((name) =>
      params.has(name)
    )
    options.seen?.(params.get("q"))
    if (params.get("q")?.endsWith("-slow")) await options.gate
    return route.fulfill({
      json: filtered
        ? { dashboards: [], next_cursor: null, total_count: 0 }
        : { dashboards: [row], next_cursor: "c2", total_count: 7 },
    })
  }
}

test("keeps the no match state while a longer search loads", async ({
  page,
}) => {
  const row = fakeSummary(uniqueTitle("Flicker"))
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  let slowSeen = false
  await page.route(
    isListRequest,
    filteredList(row, { gate, seen: (q) => (slowSeen ||= q === "zz-slow") })
  )
  const noMatch = page.getByRole("heading", {
    name: "No dashboards match these filters.",
  })
  const noDashboards = page.getByRole("heading", { name: "No dashboards yet" })

  await page.goto("/dashboards?q=zz")
  await expect(noMatch).toBeVisible()

  await filterBox(page).fill("zz-slow")
  await expect.poll(() => slowSeen).toBe(true)
  await expect(noMatch).toBeVisible()
  await page.waitForTimeout(300)
  await expect(noMatch).toBeVisible()
  await expect(noDashboards).toHaveCount(0)

  release()
  await expect(page.getByText("0 dashboards", { exact: true })).toBeVisible()
  await expect(noMatch).toBeVisible()

  await page.getByRole("button", { name: "Clear filters" }).click()
  await expect(noDashboards).toHaveCount(0)
  await expect(page.getByRole("link", { name: row.title })).toBeVisible()
})

test("hides Load more and the count while a new filter loads", async ({
  page,
}) => {
  const row = fakeSummary(uniqueTitle("Stale results"))
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  let slowSeen = false
  await page.route(
    isListRequest,
    filteredList(row, { gate, seen: (q) => (slowSeen ||= q === "x-slow") })
  )
  await page.goto("/dashboards")
  const status = page.getByRole("status").filter({ hasText: /dashboard/ })
  await expect(status).toHaveText("7 dashboards")
  await expect(page.getByRole("button", { name: "Load more" })).toBeVisible()
  await expect(dashboardsCard(page).locator("tbody")).toHaveAttribute(
    "aria-busy",
    "false"
  )

  await filterBox(page).fill("x-slow")
  await expect.poll(() => slowSeen).toBe(true)
  await expect(page.getByRole("link", { name: row.title })).toBeVisible()
  await expect(dashboardsCard(page).locator("tbody")).toHaveAttribute(
    "aria-busy",
    "true"
  )
  await expect(page.getByRole("button", { name: "Load more" })).toHaveCount(0)
  await expect(page.getByText(/^\d+ dashboards?$/)).toHaveCount(0)

  release()
  await expect(page.getByText("0 dashboards", { exact: true })).toBeVisible()
  await expect(
    page.getByRole("heading", { name: "No dashboards match these filters." })
  ).toBeVisible()
})

test("Clear filters also drops search text that was not applied yet", async ({
  page,
}) => {
  const row = fakeSummary(uniqueTitle("Clear pending"))
  await page.route(isListRequest, filteredList(row))
  await page.goto("/dashboards?datasource=no_such_ds")
  await expect(
    page.getByRole("heading", { name: "No dashboards match these filters." })
  ).toBeVisible()

  await filterBox(page).fill("typed but not applied")
  await page.getByRole("button", { name: "Clear filters" }).click()
  await expect(filterBox(page)).toHaveValue("")
  await expect(page.getByRole("link", { name: row.title })).toBeVisible()
  await page.waitForTimeout(600)
  await expect(filterBox(page)).toHaveValue("")
  expect(new URL(page.url()).search).toBe("")
})

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("fits a phone", async ({ page }) => {
    const row = fakeSummary(
      "checkout_service_p99_latency_by_region_".repeat(4),
      {
        description: "checkout_service_p99_latency_by_region_x".repeat(3),
      }
    )
    await page.route(isListRequest, (route) =>
      route.fulfill({ json: listBody([row]) })
    )
    await page.goto("/dashboards")
    await expect(page.getByRole("link", { name: row.title })).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    ).toBe(true)
    expect(
      await dashboardsCard(page).evaluate(
        (element) => element.scrollWidth > element.clientWidth
      )
    ).toBe(true)
  })
})

test.describe("on a phone with long filter values", () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test("keeps the filters and their menus inside the screen", async ({
    page,
  }) => {
    const now = new Date().toISOString()
    const person = {
      id: crypto.randomUUID(),
      name: "Bartholomew-Maximilian-Featherstonehaugh-Wolfeschlegelsteinhausenberger",
      email: "long@example.com",
      created_at: now,
      last_login_at: now,
      last_seen_at: now,
    }
    const uid = "x".repeat(40)
    await page.route(isListRequest, (route) =>
      route.fulfill({ json: listBody([fakeSummary("Phone filters")]) })
    )
    await page.route(
      (url) => url.pathname === "/api/v1/users",
      (route) => route.fulfill({ json: { users: [person], next_cursor: null } })
    )
    await page.route(
      (url) => url.pathname === "/api/v1/datasources",
      (route) =>
        route.fulfill({
          json: {
            datasources: [
              {
                uid,
                type: "prometheus",
                name: uid,
                healthy: true,
                checked_at: now,
                dashboard_count: 1,
              },
            ],
          },
        })
    )

    await page.goto(`/dashboards?made_by=user:${person.id}&datasource=${uid}`)
    const madeBy = page.getByRole("button", { name: /^Made by:/ })
    const datasource = page.getByRole("button", { name: /^Datasource:/ })
    await expect(madeBy).toContainText(person.name)
    await expect(datasource).toContainText(uid)

    const overflow = () =>
      page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth
      )
    expect(await overflow()).toBeLessThanOrEqual(0)
    for (const trigger of [madeBy, datasource]) {
      const box = await trigger.boundingBox()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(390)
    }

    for (const [trigger, option] of [
      [madeBy, person.name],
      [datasource, `${uid} (prometheus)`],
    ] as const) {
      await trigger.click()
      const item = page.getByRole("menuitemradio", { name: option })
      await expect(item).toBeVisible()
      const menu = page.getByRole("menu")
      const box = await menu.boundingBox()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(390)
      expect(
        await item.evaluate((node) => node.scrollWidth <= node.clientWidth)
      ).toBe(true)
      expect(await overflow()).toBeLessThanOrEqual(0)
      await page.keyboard.press("Escape")
      await expect(menu).toHaveCount(0)
    }
  })
})

test("shows the empty state", async ({ page }) => {
  await page.route(isListRequest, (route) =>
    route.fulfill({ json: listBody([]) })
  )
  await page.goto("/dashboards")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
  await expect(
    page.getByRole("heading", { name: "No dashboards yet" })
  ).toBeVisible()
  await expect(
    page.getByText("Ask for one in the chat and it shows up here.")
  ).toBeVisible()
  await expect(
    page.getByRole("link", { name: "Open the chat" })
  ).toHaveAttribute("href", "/")
})

function pagedList(
  pages: Record<string, { dashboards: Summary[]; next_cursor: string | null }>
) {
  return (route: Route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    const page = pages[cursor ?? ""]
    return route.fulfill({ json: listBody(page.dashboards, page.next_cursor) })
  }
}

function activeLinkTitle(page: Page) {
  return page.evaluate(
    (suffix) => (document.activeElement?.textContent ?? "").replace(suffix, ""),
    newTabSuffix
  )
}

test("loads more pages and moves focus to the first new row", async ({
  page,
}) => {
  const first = fakeSummary(uniqueTitle("Paged first"), {
    description: "alpha beta",
  })
  const secondA = fakeSummary(uniqueTitle("Paged second A"))
  const secondB = fakeSummary(uniqueTitle("Paged second B"))
  const third = fakeSummary(uniqueTitle("Paged third"))
  await page.route(
    isListRequest,
    pagedList({
      "": { dashboards: [first], next_cursor: "c2" },
      c2: { dashboards: [secondA, secondB], next_cursor: "c3" },
      c3: { dashboards: [third], next_cursor: null },
    })
  )

  await page.goto("/dashboards")
  await expect(page.getByRole("link", { name: first.title })).toBeVisible()
  await expect(page.getByText("alpha beta", { exact: true })).toHaveCount(1)
  await expect(page.getByRole("link", { name: secondA.title })).toHaveCount(0)

  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("link", { name: secondB.title })).toBeVisible()
  await expect.poll(() => activeLinkTitle(page)).toBe(secondA.title)

  await expect(loadMore).toBeVisible()
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("link", { name: third.title })).toBeVisible()
  await expect(loadMore).toHaveCount(0)
  await expect.poll(() => activeLinkTitle(page)).toBe(third.title)
})

test("keeps Load more focusable and inert while a page loads", async ({
  page,
}) => {
  const first = fakeSummary(uniqueTitle("Busy first"))
  const second = fakeSummary(uniqueTitle("Busy second"))
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  const requests: (string | null)[] = []
  await page.route(isListRequest, async (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    requests.push(cursor)
    if (cursor === "c2") await gate
    return route.fulfill({
      json: cursor === "c2" ? listBody([second]) : listBody([first], "c2"),
    })
  })

  await page.goto("/dashboards")
  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(loadMore).toHaveAttribute("aria-disabled", "true")
  await expect(loadMore).toHaveAttribute("aria-busy", "true")
  await expect(loadMore).toBeFocused()
  await expect(loadMore).not.toHaveAttribute("disabled")
  await page.keyboard.press("Enter")
  await loadMore.click({ force: true })
  await page.waitForTimeout(300)
  expect(requests.filter((cursor) => cursor === "c2")).toHaveLength(1)

  release()
  await expect(page.getByRole("link", { name: second.title })).toBeVisible()
  expect(requests.filter((cursor) => cursor === "c2")).toHaveLength(1)
  await expect.poll(() => activeLinkTitle(page)).toBe(second.title)
})

test("moves focus to the last row when Load more returns an empty page", async ({
  page,
}) => {
  const first = fakeSummary(uniqueTitle("Empty next first"))
  const second = fakeSummary(uniqueTitle("Empty next second"))
  await page.route(
    isListRequest,
    pagedList({
      "": { dashboards: [first, second], next_cursor: "c2" },
      c2: { dashboards: [], next_cursor: null },
    })
  )

  await page.goto("/dashboards")
  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(loadMore).toHaveCount(0)
  await expect.poll(() => activeLinkTitle(page)).toBe(second.title)
})

test("does not steal focus on a later refetch after a failed Load more", async ({
  page,
  consoleErrors,
}) => {
  const first = fakeSummary(uniqueTitle("Stale focus first"))
  const second = fakeSummary(uniqueTitle("Stale focus second"))
  const third = fakeSummary(uniqueTitle("Stale focus third"))
  await page.clock.install()
  let refetched = false
  await page.route(isListRequest, (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    if (cursor === "c2") {
      return route.fulfill({
        status: 500,
        json: { error: "page boom", code: "internal" },
      })
    }
    return route.fulfill({
      json: refetched
        ? listBody([first, second, third])
        : listBody([first, second], "c2"),
    })
  })

  await page.goto("/dashboards")
  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("alert")).toContainText("page boom")
  await expect(loadMore).toBeFocused()

  await page.evaluate(() => (document.activeElement as HTMLElement).blur())
  refetched = true
  await page.clock.fastForward(60_000)
  await page.evaluate(() => {
    dispatchEvent(new Event("offline"))
    dispatchEvent(new Event("online"))
  })
  await expect(page.getByRole("link", { name: third.title })).toBeVisible()
  expect(
    await page.evaluate(() => document.activeElement === document.body)
  ).toBe(true)
  consoleErrors.length = 0
})

test("shows a failed Load more next to the button and retries from it", async ({
  page,
  consoleErrors,
}) => {
  const first = fakeSummary(uniqueTitle("Failed first"))
  const second = fakeSummary(uniqueTitle("Failed second"))
  let failNext = true
  await page.route(isListRequest, (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    if (cursor !== "c2") {
      return route.fulfill({ json: listBody([first], "c2") })
    }
    if (failNext) {
      failNext = false
      return route.fulfill({
        status: 500,
        json: { error: "page boom", code: "internal" },
      })
    }
    return route.fulfill({ json: listBody([second]) })
  })

  await page.goto("/dashboards")
  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.click()

  const inline = page.getByRole("alert")
  await expect(inline).toHaveCount(1)
  await expect(inline).toContainText("page boom")
  await expect(page.getByText("Could not load dashboards")).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Retry" })).toHaveCount(0)
  await expect(page.getByRole("link", { name: first.title })).toBeVisible()
  await expect(inline).toBeInViewport()
  const errorBox = await inline.boundingBox()
  const buttonBox = await loadMore.boundingBox()
  expect(Math.abs(errorBox!.y - buttonBox!.y)).toBeLessThan(80)
  await expect(loadMore).toBeEnabled()

  await loadMore.click()
  await expect(page.getByRole("link", { name: second.title })).toBeVisible()
  await expect(page.getByRole("alert")).toHaveCount(0)
  await expect(loadMore).toHaveCount(0)
  await expect.poll(() => activeLinkTitle(page)).toBe(second.title)
  consoleErrors.length = 0
})

test("shows an error with retry", async ({ page, request, consoleErrors }) => {
  const title = uniqueTitle("List retry")
  await createDashboard(request, { ...loadSpec("overview"), title })

  const failing = (url: URL) => isListRequest(url)
  await page.route(failing, (route) =>
    route.fulfill({
      status: 500,
      json: { error: "boom", code: "internal" },
    })
  )
  await page.goto("/dashboards")
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("Boom.")
  const retry = page.getByRole("button", { name: "Retry" })
  await expect(retry).toHaveCSS("color", await foregroundColor(page))

  await page.unroute(failing)
  await retry.click()
  await expect(page.getByRole("link", { name: title })).toBeVisible()
  await expect(alert).toHaveCount(0)
  consoleErrors.length = 0
})

function foregroundColor(page: Page) {
  return page.evaluate(() => getComputedStyle(document.body).color)
}

test("keeps the empty state and a busy Retry when a refetch fails", async ({
  page,
  consoleErrors,
}) => {
  await page.clock.install()
  let mode: "empty" | "fail" | "slow" = "empty"
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  let attempts = 0
  await page.route(isListRequest, async (route) => {
    if (mode === "fail") {
      return route.fulfill({
        status: 500,
        json: { error: "boom", code: "internal" },
      })
    }
    if (mode === "slow") {
      attempts += 1
      await gate
    }
    return route.fulfill({ json: listBody([]) })
  })
  const emptyHeading = page.getByRole("heading", { name: "No dashboards yet" })
  await page.goto("/dashboards")
  await expect(emptyHeading).toBeVisible()

  mode = "fail"
  await page.clock.fastForward(60_000)
  await page.evaluate(() => {
    history.pushState(null, "", "/no/such/page")
    dispatchEvent(new PopStateEvent("popstate"))
  })
  await page.getByRole("link", { name: "Back to home" }).click()
  await page
    .getByRole("navigation", { name: "Main" })
    .getByRole("link", { name: "Dashboards" })
    .click()
  await expect(page.getByRole("alert")).toContainText("Boom.")
  await expect(emptyHeading).toBeVisible()
  await expect(page.getByRole("table")).toHaveCount(0)

  mode = "slow"
  const retry = page.getByRole("button", { name: "Retry" })
  await retry.click()
  await expect(retry).toHaveAttribute("aria-disabled", "true")
  await expect(retry).toBeFocused()
  await page.keyboard.press("Enter")
  await page.waitForTimeout(300)
  expect(attempts).toBe(1)

  release()
  await expect(page.getByRole("alert")).toHaveCount(0)
  expect(attempts).toBe(1)
  await expect(emptyHeading).toBeVisible()
  consoleErrors.length = 0
})
