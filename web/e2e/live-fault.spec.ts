import { execFileSync } from "node:child_process"
import { join } from "node:path"

import type { APIRequestContext } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

const astronomyScript = join(
  import.meta.dirname,
  "..",
  "..",
  "scripts",
  "astronomy.sh"
)
const shopUrl = `http://localhost:${process.env.ASTRONOMY_SHOP_HOST_PORT ?? "18081"}`
const faultFlag = "cartFailure"

const errorRateWindowDecayMs = 240_000
const trafficIntervalMs = 3_000

function setFlag(variant: string) {
  execFileSync(astronomyScript, ["flag", faultFlag, variant], {
    stdio: "pipe",
  })
}

function startTraffic(request: APIRequestContext) {
  let running = true
  const loop = (async () => {
    while (running) {
      await request
        .post(`${shopUrl}/api/cart`, {
          data: {
            item: { productId: "OLJCESPC7Z", quantity: 1 },
            userId: `e2e-${Date.now()}`,
          },
          timeout: 30_000,
        })
        .catch(() => undefined)
      await new Promise((resolve) => setTimeout(resolve, trafficIntervalMs))
    }
  })()
  return async () => {
    running = false
    await loop
  }
}

test.afterEach(() => {
  setFlag("off")
})

test("an injected fault turns the error rate critical", async ({
  page,
  request,
}) => {
  test.setTimeout(30_000 + errorRateWindowDecayMs + 240_000 + 30_000)
  const stopTraffic = startTraffic(request)
  try {
    const spec = loadSpec("fault")
    spec.title = uniqueTitle("E2E live fault")
    const { id } = await createDashboard(request, spec)
    await openDashboard(page, id)

    const article = panel(page, "Cart error rate")
    const tile = article.getByRole("group")
    await expect(tile).toBeVisible({ timeout: 30_000 })
    await expect(tile).toHaveAttribute("data-threshold", "base", {
      timeout: errorRateWindowDecayMs,
    })

    setFlag("100%")

    await expect(tile).toHaveAttribute("data-threshold", "critical", {
      timeout: 240_000,
    })
    await expect(article.getByText("Critical", { exact: true })).toBeVisible()
  } finally {
    await stopTraffic()
  }
})
