import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const shopUrl = `http://localhost:${process.env.SLASHOBSERVE_SHOP_HOST_PORT ?? "18081"}`

test.afterEach(async ({ request }) => {
  await request.delete(`${shopUrl}/fault`).catch(() => undefined)
})

test("an injected fault turns the error rate critical", async ({
  page,
  request,
}) => {
  test.setTimeout(150_000)
  const spec = loadSpec("fault")
  spec.title = uniqueTitle("E2E live fault")
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)

  const article = panel(page, "checkout-0 error rate")
  const tile = article.getByRole("group")
  await expect(tile).toBeVisible({ timeout: 30_000 })
  await expect(tile).toHaveAttribute("data-threshold", "base", {
    timeout: 60_000,
  })

  const injected = await request.post(`${shopUrl}/fault?type=errors&pct=100`)
  expect(injected.status()).toBe(200)

  await expect(tile).toHaveAttribute("data-threshold", "critical", {
    timeout: 90_000,
  })
  await expect(article.getByText("Critical", { exact: true })).toBeVisible()
})
