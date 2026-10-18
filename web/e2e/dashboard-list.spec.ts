import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expect, test } from "./fixtures"

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

  await linkB.click()
  await expect(page).toHaveURL(new RegExp(`/dashboards/${b.id}`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(titleB)
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

test("loads more pages", async ({ page }) => {
  const first = fakeSummary(uniqueTitle("Paged first"), ["alpha", "beta"])
  const second = fakeSummary(uniqueTitle("Paged second"))
  await page.route(
    (url) => isListRequest(url),
    (route) => {
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      return route.fulfill({
        json:
          cursor === "c2"
            ? { dashboards: [second], next_cursor: null }
            : { dashboards: [first], next_cursor: "c2" },
      })
    }
  )

  await page.goto("/")
  await expect(page.getByRole("link", { name: first.title })).toBeVisible()
  await expect(page.getByText("alpha")).toBeVisible()
  await expect(page.getByText("beta")).toBeVisible()
  await expect(page.getByRole("link", { name: second.title })).toHaveCount(0)

  const loadMore = page.getByRole("button", { name: "Load more" })
  await loadMore.click()
  await expect(page.getByRole("link", { name: second.title })).toBeVisible()
  await expect(page.getByRole("link", { name: first.title })).toBeVisible()
  await expect(loadMore).toHaveCount(0)
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

  await page.unroute(failing)
  await page.getByRole("button", { name: "Retry" }).click()
  await expect(page.getByRole("link", { name: title })).toBeVisible()
  await expect(alert).toHaveCount(0)
  consoleErrors.length = 0
})
