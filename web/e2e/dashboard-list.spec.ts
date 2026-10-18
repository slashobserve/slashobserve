import { createDashboard, loadSpec, uniqueTitle } from "./api"
import type { Route } from "@playwright/test"

import { expect, test, type Page } from "./fixtures"

const listPath = "/api/v1/dashboards"

function isListRequest(url: URL): boolean {
  return url.pathname === listPath
}

function fakeSummary(title: string, tags: string[] = []) {
  const now = new Date().toISOString()
  return {
    id: crypto.randomUUID(),
    title,
    tags,
    latest_version: 3,
    created_at: now,
    updated_at: now,
    updated_by: {
      kind: "user",
      id: crypto.randomUUID(),
      name: "Fake Author",
    },
  }
}

test("lists dashboards newest first and opens one", async ({
  page,
  request,
}) => {
  const titleA = uniqueTitle("List A")
  const titleB = uniqueTitle("List B")
  await createDashboard(request, { ...loadSpec("overview"), title: titleA })
  const b = await createDashboard(request, {
    ...loadSpec("overview"),
    title: titleB,
  })

  await page.goto("/")
  const linkA = page.getByRole("link", { name: titleA })
  const linkB = page.getByRole("link", { name: titleB })
  await expect(linkA).toBeVisible()
  await expect(linkB).toBeVisible()

  const boxA = await linkA.boundingBox()
  const boxB = await linkB.boundingBox()
  expect(boxB!.y).toBeLessThan(boxA!.y)

  for (const link of [linkA, linkB]) {
    const row = page.getByRole("row").filter({ has: link })
    await expect(row.getByText("v1", { exact: true })).toBeVisible()
  }

  await expect(page.getByRole("table", { name: "Dashboards" })).toBeVisible()

  await linkB.click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${b.id}`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(titleB)
})

test("keeps unbroken titles and tags inside the table on a phone", async ({
  page,
}) => {
  const row = fakeSummary("checkout_service_p99_latency_by_region_".repeat(4), [
    "checkout_service_p99_latency_by_region_x",
  ])
  await page.route(isListRequest, (route) =>
    route.fulfill({ json: { dashboards: [row], next_cursor: null } })
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/")
  await expect(page.getByRole("link", { name: row.title })).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})

test("shows the empty state", async ({ page }) => {
  await page.route(
    (url) => isListRequest(url),
    (route) => route.fulfill({ json: { dashboards: [], next_cursor: null } })
  )
  await page.goto("/")
  await expect(
    page.getByRole("heading", { level: 1, name: "Dashboards" })
  ).toBeVisible()
  await expect(
    page.getByRole("heading", { name: "No dashboards yet" })
  ).toBeVisible()
  await expect(
    page.getByText("Dashboards you or the agent create appear here.")
  ).toBeVisible()
})

function pagedList(
  pages: Record<string, { dashboards: object[]; next_cursor: string | null }>
) {
  return (route: Route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")
    return route.fulfill({ json: pages[cursor ?? ""] })
  }
}

function activeElementText(page: Page) {
  return page.evaluate(() => document.activeElement?.textContent ?? "")
}

test("loads more pages and moves focus to the first new row", async ({
  page,
}) => {
  const first = fakeSummary(uniqueTitle("Paged first"), ["alpha", "beta"])
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

  await page.goto("/")
  await expect(page.getByRole("link", { name: first.title })).toBeVisible()
  await expect(page.getByText("alpha", { exact: true })).toHaveCount(1)
  await expect(page.getByText("beta", { exact: true })).toHaveCount(1)
  await expect(page.getByRole("link", { name: secondA.title })).toHaveCount(0)

  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("link", { name: secondB.title })).toBeVisible()
  await expect.poll(() => activeElementText(page)).toBe(secondA.title)

  await expect(loadMore).toBeVisible()
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("link", { name: third.title })).toBeVisible()
  await expect(loadMore).toHaveCount(0)
  await expect.poll(() => activeElementText(page)).toBe(third.title)
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
      json:
        cursor === "c2"
          ? { dashboards: [second], next_cursor: null }
          : { dashboards: [first], next_cursor: "c2" },
    })
  })

  await page.goto("/")
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
  await expect.poll(() => activeElementText(page)).toBe(second.title)
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

  await page.goto("/")
  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.focus()
  await page.keyboard.press("Enter")
  await expect(loadMore).toHaveCount(0)
  await expect.poll(() => activeElementText(page)).toBe(second.title)
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
      json: {
        dashboards: refetched ? [first, second, third] : [first, second],
        next_cursor: refetched ? null : "c2",
      },
    })
  })

  await page.goto("/")
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
      return route.fulfill({
        json: { dashboards: [first], next_cursor: "c2" },
      })
    }
    if (failNext) {
      failNext = false
      return route.fulfill({
        status: 500,
        json: { error: "page boom", code: "internal" },
      })
    }
    return route.fulfill({ json: { dashboards: [second], next_cursor: null } })
  })

  await page.goto("/")
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
  await expect.poll(() => activeElementText(page)).toBe(second.title)
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
  await page.goto("/")
  const alert = page.getByRole("alert")
  await expect(alert).toContainText("boom")
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
    return route.fulfill({ json: { dashboards: [], next_cursor: null } })
  })
  const emptyHeading = page.getByRole("heading", { name: "No dashboards yet" })
  await page.goto("/")
  await expect(emptyHeading).toBeVisible()

  mode = "fail"
  await page.clock.fastForward(60_000)
  await page.evaluate(() => {
    history.pushState(null, "", "/no/such/page")
    dispatchEvent(new PopStateEvent("popstate"))
  })
  await page.getByRole("link", { name: "Back to home" }).click()
  await expect(page.getByRole("alert")).toContainText("boom")
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
