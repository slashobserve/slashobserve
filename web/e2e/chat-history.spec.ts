import type { Locator, Route } from "@playwright/test"

import { mutationHeaders } from "./api"
import { expect, test, type Page } from "./fixtures"

const neverSavesPrompt = "e2e: a slow run that never saves"
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"

const shortWindow = { width: 1440, height: 380 }

type ListedRequest = ReturnType<typeof listedRequest>

async function createRequest(page: Page, prompt: string): Promise<string> {
  const created = await page.request.post("/api/v1/observe", {
    data: { prompt },
    headers: mutationHeaders,
  })
  expect(created.status()).toBe(202)
  const { request } = (await created.json()) as { request: { id: string } }
  return request.id
}

function historyButton(page: Page) {
  return page.getByRole("button", { name: "History", exact: true })
}

function panel(page: Page) {
  return page.getByRole("complementary", { name: "History" })
}

function rowsOf(aside: Locator) {
  return aside.getByRole("list").getByRole("button")
}

async function openHistory(page: Page, path = "/") {
  await page.goto(path)
  await historyButton(page).click()
  await expect(historyButton(page)).toHaveAttribute("aria-expanded", "true")
  await expect(panel(page)).toBeVisible()
}

function listedRequest(label: string, index: number) {
  return {
    id: `00000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
    source: "prompt" as const,
    prompt: `e2e listed request ${label} ${index}: a long prompt that wraps onto a second line in the History card`,
    status: "succeeded" as const,
    dashboard_id: null,
    dashboard_title: null,
    panel_id: null,
    result_version: 1,
    url: null,
    error: null,
    origin: "web" as const,
    origin_client: null,
    created_at: new Date(Date.now() - index * 60_000).toISOString(),
    started_at: null,
    finished_at: null,
  }
}

function listedRequests(label: string, count: number, offset = 0) {
  return Array.from({ length: count }, (_, index) =>
    listedRequest(label, offset + index)
  )
}

function historyList(requests: ListedRequest[], nextCursor: string | null) {
  return { requests, next_cursor: nextCursor }
}

function isHistoryList(url: URL) {
  return url.pathname === "/api/v1/observe" && !url.searchParams.has("source")
}

async function mockHistoryList(
  page: Page,
  handler: (cursor: string | null, route: Route) => Promise<void> | void
) {
  await page.route(isHistoryList, (route) => {
    if (route.request().method() !== "GET") return route.fallback()
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    return handler(cursor, route)
  })
}

async function failedListLogged(consoleErrors: string[], count: number) {
  const failures = () =>
    consoleErrors.filter((message) => message.includes("status of 500")).length
  await expect.poll(failures).toBe(count)
  consoleErrors.length = 0
}

async function pageScroll(page: Page) {
  return page.evaluate(() => {
    const root = document.scrollingElement as Element
    return {
      scrollTop: root.scrollTop,
      overflow: root.scrollHeight - window.innerHeight,
    }
  })
}

async function bodyOverflow(aside: Locator) {
  return aside.evaluate((element) => {
    for (const candidate of element.querySelectorAll("div")) {
      if (getComputedStyle(candidate).overflowY === "auto") {
        return candidate.scrollHeight - candidate.clientHeight
      }
    }
    return 0
  })
}

async function bodyScrollTop(aside: Locator) {
  return aside.evaluate((element) => {
    for (const candidate of element.querySelectorAll("div")) {
      if (getComputedStyle(candidate).overflowY === "auto") {
        return candidate.scrollTop
      }
    }
    return 0
  })
}

async function scrollBodyToEnd(aside: Locator) {
  await aside.evaluate((element) => {
    for (const candidate of element.querySelectorAll("div")) {
      if (getComputedStyle(candidate).overflowY === "auto") {
        candidate.scrollTop = candidate.scrollHeight
      }
    }
  })
}

for (const scheme of ["light", "dark"] as const) {
  test.describe(scheme, () => {
    test.use({ colorScheme: scheme })

    test("opens History from the top row and reopens a chat", async ({
      page,
    }) => {
      await createRequest(page, neverSavesPrompt)
      const newerId = await createRequest(page, unanswerablePrompt)

      await page.goto("/")
      const button = historyButton(page)
      await expect(button).toHaveAttribute("aria-expanded", "false")
      await button.click()
      await expect(button).toHaveAttribute("aria-expanded", "true")

      const aside = panel(page)
      await expect(aside).toBeVisible()
      await expect(
        aside.getByRole("heading", { level: 2, name: "History" })
      ).toBeFocused()
      const rows = rowsOf(aside)
      await expect(rows.nth(0)).toContainText(unanswerablePrompt)
      await expect(rows.nth(1)).toContainText(neverSavesPrompt)

      await rows.nth(0).click()
      await expect(page).toHaveURL(new RegExp(`/observe/${newerId}$`))
      await expect(aside).toBeVisible()
      await expect(button).toHaveAttribute("aria-expanded", "true")
      await expect(rows.nth(0)).toHaveAttribute("aria-current", "true")
      await expect(rows.nth(1)).not.toHaveAttribute("aria-current", "true")
    })

    test("shows a failed chat only in History", async ({ page }) => {
      test.setTimeout(120_000)
      await createRequest(page, unanswerablePrompt)
      await openHistory(page)

      const row = rowsOf(panel(page)).filter({ hasText: unanswerablePrompt })
      await expect(row.first().locator('[data-status="critical"]')).toBeVisible(
        { timeout: 90_000 }
      )
    })

    test("closes with Escape from a focused row and returns focus", async ({
      page,
    }) => {
      await createRequest(page, unanswerablePrompt)
      await openHistory(page)

      await rowsOf(panel(page)).first().focus()
      await page.keyboard.press("Escape")
      await expect(panel(page)).toHaveCount(0)
      await expect(historyButton(page)).toBeFocused()
      await expect(historyButton(page)).toHaveAttribute(
        "aria-expanded",
        "false"
      )
    })

    for (const closer of ["Escape", "Close"] as const) {
      test(`returns focus to History when ${closer} closes the card after opening a chat from /`, async ({
        page,
      }) => {
        const id = await createRequest(page, unanswerablePrompt)
        await openHistory(page)

        await rowsOf(panel(page)).first().click()
        await expect(page).toHaveURL(new RegExp(`/observe/${id}$`))
        await expect(panel(page)).toBeVisible()
        await expect(
          page.getByRole("heading", { level: 1, name: unanswerablePrompt })
        ).toBeFocused()

        if (closer === "Escape") {
          await rowsOf(panel(page)).first().focus()
          await page.keyboard.press("Escape")
        } else {
          await panel(page).getByRole("button", { name: "Close" }).click()
        }
        await expect(panel(page)).toHaveCount(0)
        await expect(historyButton(page)).toBeFocused()
      })
    }

    test("returns focus to the composer when Escape closes the card reopened on the chat home", async ({
      page,
    }) => {
      await openHistory(page)
      const trigger = page.getByRole("button", { name: "Search or jump to" })
      const palette = page.getByRole("dialog", { name: "Search or jump to" })

      await trigger.click()
      await palette
        .getByRole("option", { name: "Dashboards", exact: true })
        .click()
      await expect(page).toHaveURL(/\/dashboards$/)
      await expect(historyButton(page)).toHaveCount(0)

      await trigger.click()
      await palette
        .getByRole("option", { name: "Observe", exact: true })
        .click()
      await expect(page).toHaveURL(/\/$/)
      await expect(panel(page)).toBeVisible()

      await panel(page).getByRole("heading", { level: 2 }).focus()
      await page.keyboard.press("Escape")
      await expect(panel(page)).toHaveCount(0)
      await expect(
        page.getByRole("textbox", { name: "Message the agent" })
      ).toBeFocused()
    })

    test("is a card, not a dialog", async ({ page }) => {
      await openHistory(page)
      await expect(page.getByRole("dialog")).toHaveCount(0)
      const edges = await panel(page).evaluate((element) => {
        const style = getComputedStyle(element)
        return {
          boxShadow: style.boxShadow,
          borderLeftWidth: style.borderLeftWidth,
        }
      })
      expect(edges).toEqual({ boxShadow: "none", borderLeftWidth: "1px" })
    })

    test("keeps Load more focused while loading and moves focus to the first new row", async ({
      page,
      consoleErrors,
    }) => {
      const first = listedRequests("first", 2)
      const second = listedRequests("second", 2, 2)
      let release!: () => void
      const gate = new Promise<void>((resolve) => (release = resolve))
      let failNext = true
      await mockHistoryList(page, async (cursor, route) => {
        if (cursor === null) {
          return route.fulfill({ json: historyList(first, "c2") })
        }
        if (failNext) {
          failNext = false
          return route.fulfill({
            status: 500,
            json: { error: "page boom", code: "internal" },
          })
        }
        await gate
        return route.fulfill({ json: historyList(second, null) })
      })

      await openHistory(page)
      const aside = panel(page)
      const rows = rowsOf(aside)
      await expect(rows).toHaveCount(2)
      const loadMore = aside.getByRole("button", { name: "Load more" })

      await loadMore.focus()
      await page.keyboard.press("Enter")
      const alert = aside.getByRole("alert")
      await expect(alert).toHaveText("Could not load more requests.")
      await expect(loadMore).toBeFocused()
      await expect(loadMore).toHaveAttribute(
        "aria-describedby",
        (await alert.getAttribute("id")) ?? ""
      )
      await expect(rows).toHaveCount(2)
      await failedListLogged(consoleErrors, 1)

      await page.keyboard.press("Enter")
      await expect(loadMore).toHaveAttribute("aria-busy", "true")
      await expect(loadMore).toBeFocused()
      release()
      await expect(rows).toHaveCount(4)
      await expect(rows.nth(2)).toBeFocused()
      await expect(rows.nth(2)).toContainText("second 2")
      await expect(loadMore).toHaveCount(0)
      await expect(alert).toHaveCount(0)
    })

    test("moves focus to Retry after a failed retry and to the first row after a successful one", async ({
      page,
      consoleErrors,
    }) => {
      const requests = listedRequests("retry", 3)
      let failures = 2
      await mockHistoryList(page, (_cursor, route) => {
        if (failures > 0) {
          failures -= 1
          return route.fulfill({
            status: 500,
            json: { error: "list boom", code: "internal" },
          })
        }
        return route.fulfill({ json: historyList(requests, null) })
      })

      await openHistory(page)
      const aside = panel(page)
      await expect(
        aside.getByText("Could not load your requests.")
      ).toBeVisible()
      const retry = aside.getByRole("button", { name: "Retry" })

      await retry.focus()
      await page.keyboard.press("Enter")
      await expect.poll(() => failures).toBe(0)
      await expect(
        aside.getByText("Could not load your requests.")
      ).toBeVisible()
      await expect(retry).toBeFocused()
      await failedListLogged(consoleErrors, 2)

      await page.keyboard.press("Enter")
      const rows = rowsOf(aside)
      await expect(rows).toHaveCount(3)
      await expect(rows.nth(0)).toBeFocused()
    })

    test.describe("on a phone", () => {
      test.use({ viewport: { width: 390, height: 844 } })

      test("fits a phone", async ({ page }) => {
        await openHistory(page)
        await expect(historyButton(page)).toBeVisible()
        const fits = await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
        expect(fits).toBe(true)
      })
    })
  })
}

test.describe("on a short window", () => {
  test.use({ viewport: shortWindow })

  test("keeps the History head in view and the page still when the list overflows", async ({
    page,
  }) => {
    await mockHistoryList(page, (_cursor, route) =>
      route.fulfill({ json: historyList(listedRequests("tall", 20), null) })
    )
    await openHistory(page)

    const aside = panel(page)
    const rows = rowsOf(aside)
    await expect(rows).toHaveCount(20)

    await scrollBodyToEnd(aside)
    await expect(rows.last()).toBeInViewport()
    await expect(
      aside.getByRole("heading", { level: 2, name: "History" })
    ).toBeInViewport()
    await expect(aside.getByRole("button", { name: "Close" })).toBeInViewport()
    const box = await aside.boundingBox()
    expect(box?.y).toBe(0)
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(
      shortWindow.height
    )
    expect(await pageScroll(page)).toEqual({ scrollTop: 0, overflow: 0 })
    expect(await bodyOverflow(aside)).toBeGreaterThan(0)
  })

  test("keeps the list scroll position when a row opens a chat from the chat home", async ({
    page,
  }) => {
    const id = await createRequest(page, unanswerablePrompt)
    const real = {
      ...listedRequest("real", 19),
      id,
      prompt: unanswerablePrompt,
    }
    await mockHistoryList(page, (_cursor, route) =>
      route.fulfill({
        json: historyList([...listedRequests("tall", 19), real], null),
      })
    )
    await openHistory(page)

    const aside = panel(page)
    const rows = rowsOf(aside)
    await expect(rows).toHaveCount(20)
    await scrollBodyToEnd(aside)
    const scrolled = await bodyScrollTop(aside)
    expect(scrolled).toBeGreaterThan(0)

    await rows.last().click()
    await expect(page).toHaveURL(new RegExp(`/observe/${id}$`))
    await expect(
      page.getByRole("heading", { level: 1, name: unanswerablePrompt })
    ).toBeVisible()
    await expect(rows.last()).toHaveAttribute("aria-current", "true")
    expect(await bodyScrollTop(aside)).toBe(scrolled)
  })

  test("keeps the Create key head and footer in view and the page still", async ({
    page,
  }) => {
    await page.goto("/keys")
    await page
      .getByRole("main")
      .getByRole("button", { name: "Create key", exact: true })
      .first()
      .click()

    const aside = page.getByRole("complementary", { name: "Create key" })
    await expect(aside.getByLabel("Name", { exact: true })).toBeVisible()
    await aside.getByRole("combobox", { name: "Service account" }).click()
    await page
      .getByRole("option", { name: "New service account", exact: true })
      .click()
    await expect(aside.getByLabel("Service account name")).toBeVisible()

    await scrollBodyToEnd(aside)
    await expect(
      aside.getByRole("heading", { level: 2, name: "Create key" })
    ).toBeInViewport()
    await expect(aside.getByRole("button", { name: "Close" })).toBeInViewport()
    await expect(aside.getByRole("button", { name: "Cancel" })).toBeInViewport()
    await expect(
      aside.getByRole("button", { name: "Create key", exact: true })
    ).toBeInViewport()
    expect(await pageScroll(page)).toEqual({ scrollTop: 0, overflow: 0 })
    expect(await bodyOverflow(aside)).toBeGreaterThan(0)
  })
})
