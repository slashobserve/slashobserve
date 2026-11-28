import { createDashboard, loadSpec, uniqueTitle } from "./api"

import { expect, test, type Page } from "./fixtures"

const folderRows = 1

function dashboardsCard(page: Page) {
  return page.getByRole("grid", { name: "Dashboards" })
}

function filterBox(page: Page) {
  return page.getByRole("textbox", { name: "Filter dashboards" })
}

function rowFor(page: Page, title: string) {
  return page
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: title, exact: true }) })
}

async function filterByTitle(page: Page, title: string) {
  await filterBox(page).fill(title)
  await expect(rowFor(page, title)).toBeVisible()
  await expect(dashboardsCard(page).locator("[row-index]")).toHaveCount(
    folderRows + 1
  )
}

function searchParam(page: Page, name: string) {
  return new URL(page.url()).searchParams.get(name)
}

test("opens a dashboard in the same tab and from the arrow in a new tab", async ({
  page,
  context,
  request,
}) => {
  const title = uniqueTitle("List tab")
  const created = await createDashboard(request, {
    ...loadSpec("overview"),
    title,
  })
  await page.goto("/dashboards")
  await filterByTitle(page, title)

  const link = page.getByRole("link", { name: title, exact: true })
  await expect(link).not.toHaveAttribute("target")
  const arrow = page.getByRole("link", { name: `Open ${title} in a new tab` })
  await expect(arrow).toHaveAttribute("target", "_blank")
  await expect(arrow).toHaveCSS("opacity", "0")
  await link.hover()
  await expect(arrow).toHaveCSS("opacity", "1")
  const [popup] = await Promise.all([
    context.waitForEvent("page"),
    arrow.click(),
  ])
  await expect(popup).toHaveURL(new RegExp(`/d/${created.id}`))
  await expect(popup.getByRole("heading", { level: 1 })).toHaveText(title)
  await expect(page).toHaveURL(/\/dashboards(\?.*)?$/)
  await popup.close()

  await link.click()
  await expect(page).toHaveURL(new RegExp(`/d/${created.id}`))
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title)
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
