import type { Locator, Page } from "@playwright/test"

import { card } from "./activity"
import { expectPanelsReady } from "./dashboards"
import { expect, test } from "./fixtures"

const agentTimeout = 90_000
const askPrompt = "demo shop RED metrics by endpoint"
const followUpPrompt = "Show this per route"
const requestPath = /\/observe\/([0-9a-f-]{36})$/
const dashboardPath = /\/dashboards\/[0-9a-f-]{36}$/

function turnWith(page: Page, prompt: string) {
  return page
    .getByRole("region", { name: /^Request / })
    .filter({ has: page.getByText(prompt, { exact: true }) })
}

function composer(page: Page) {
  return page.getByRole("textbox", { name: "Message the agent" })
}

function historyRows(page: Page): Locator {
  return page
    .getByRole("complementary", { name: "History" })
    .getByRole("list")
    .getByRole("button")
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })
    test.setTimeout(240_000)

    test("asks for a dashboard, opens it, follows up and finds the chat again", async ({
      page,
    }) => {
      let firstId = ""
      const firstTurn = turnWith(page, askPrompt)
      const secondTurn = turnWith(page, followUpPrompt)

      await test.step("ask", async () => {
        await page.goto("/")
        await composer(page).fill(askPrompt)
        await composer(page).press("Enter")

        await expect(page).toHaveURL(requestPath)
        firstId = requestPath.exec(page.url())?.[1] ?? ""
        expect(firstId).not.toBe("")
      })

      await test.step("watch the steps", async () => {
        await expect(firstTurn).toBeVisible()
        await expect(firstTurn.getByText(/Saved as version 1/)).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(
          firstTurn.getByRole("link", { name: /^Open dashboard/ })
        ).toBeVisible()

        await firstTurn.getByText(/^Ran \d+ steps?/).click()
        const steps = firstTurn
          .getByRole("list", { name: "Progress" })
          .getByRole("listitem")
        await expect(steps.first()).toBeVisible()
        await expect(steps.filter({ hasText: /^Saving/ }).first()).toBeVisible()
      })

      await test.step("open the dashboard", async () => {
        const [tab] = await Promise.all([
          page.context().waitForEvent("page"),
          firstTurn.getByRole("link", { name: /^Open dashboard/ }).click(),
        ])
        await expect(tab).toHaveURL(dashboardPath)
        await expect(tab.getByRole("heading", { level: 1 })).not.toHaveText("")
        await expectPanelsReady(tab)
        await expect(
          tab.locator("article[data-panel-id]").first()
        ).toBeVisible()
        await expect(card(tab)).toBeVisible()
        await tab.close()
      })

      await test.step("follow up", async () => {
        await composer(page).fill(followUpPrompt)
        await composer(page).press("Enter")

        await expect(secondTurn).toBeVisible()
        await expect(secondTurn.getByText(/Saved as version 2/)).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(page).toHaveURL(new RegExp(`/observe/${firstId}$`))
        const turns = page.getByRole("region", { name: /^Request / })
        await expect(turns.first()).toContainText(askPrompt)
        await expect(turns.last()).toContainText(followUpPrompt)
      })

      await test.step("find it in History", async () => {
        const history = page.getByRole("button", {
          name: "History",
          exact: true,
        })
        await history.click()
        await expect(
          page.getByRole("complementary", { name: "History" })
        ).toBeVisible()

        const rows = historyRows(page)
        const current = rows.and(page.locator('[aria-current="true"]'))
        await expect(current).toHaveCount(1)
        await expect(current).toContainText(askPrompt)
        const order = await rows.evaluateAll(
          (buttons, followUp) => ({
            followUp: buttons.findIndex((button) =>
              button.textContent?.includes(followUp)
            ),
            current: buttons.findIndex(
              (button) => button.getAttribute("aria-current") === "true"
            ),
          }),
          followUpPrompt
        )
        expect(order.followUp).toBeGreaterThanOrEqual(0)
        expect(order.followUp).toBeLessThan(order.current)

        await rows.nth(order.followUp).click()
        await expect(page).toHaveURL(requestPath)
        expect(page.url()).not.toContain(firstId)
        await expect(rows.nth(order.followUp)).toHaveAttribute(
          "aria-current",
          "true"
        )

        await rows.nth(order.current).click()
        await expect(page).toHaveURL(new RegExp(`/observe/${firstId}$`))
        await expect(rows.nth(order.current)).toHaveAttribute(
          "aria-current",
          "true"
        )
      })

      await test.step("find it in Recent chats", async () => {
        await page.goto("/")
        const recent = page.getByRole("region", { name: "Recent chats" })
        const row = recent
          .getByRole("button", { name: new RegExp(askPrompt) })
          .first()
        await expect(row).toBeVisible()
        await row.click()
        await expect(page).toHaveURL(new RegExp(`/observe/${firstId}$`))
      })

      await test.step("theme", async () => {
        await expect(page.locator("html")).toHaveClass(
          colorScheme === "dark" ? /dark/ : /^(?!.*dark)/
        )
      })
    })
  })
}
