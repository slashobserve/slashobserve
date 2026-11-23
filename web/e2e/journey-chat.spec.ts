import type { Locator, Page } from "@playwright/test"

import { card } from "./activity"
import {
  holdRequestPollsUntilFinished,
  reachDashboard,
  sentRequestId,
} from "./builds"
import { expectPanelsReady } from "./dashboards"
import { expect, test } from "./fixtures"

const agentTimeout = 90_000
const askPrompt = "astronomy shop RED metrics by endpoint"
const followUpQuestion = "Can this panel show each route?"
const followUpPrompt = "Show this per route"
const requestPath = /\/c\/([0-9a-f-]{36})$/
const dashboardTitle = "Astronomy shop RED metrics"
const refinedDashboardTitle = "E2E dashboard"

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
    .locator("[data-chat-row]")
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })
    test.setTimeout(240_000)

    test("asks for a dashboard, opens it, follows up and finds the chat again", async ({
      page,
    }) => {
      let firstId = ""
      let dashboardId = ""
      const firstTurn = turnWith(page, askPrompt)
      const questionTurn = turnWith(page, followUpQuestion)
      const release = await holdRequestPollsUntilFinished(page)

      await test.step("ask", async () => {
        await page.goto("/ask")
        await expect(
          page.getByRole("heading", {
            level: 1,
            name: "What do you want to see?",
          })
        ).toBeVisible()
        await composer(page).fill(askPrompt)
        await composer(page).press("Enter")

        firstId = await sentRequestId(page)
        expect(firstId).not.toBe("")
      })

      await test.step("watch the build open the dashboard", async () => {
        await expect(firstTurn).toBeVisible()
        await release(firstId)
        await reachDashboard(page, agentTimeout)
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(
          dashboardTitle
        )
        await expectPanelsReady(page)
      })

      await test.step("watch the steps", async () => {
        await page.goto(`/c/${firstId}`)
        await expect(firstTurn).toBeVisible()
        await expect(firstTurn.getByText(/Saved as version 1/)).toBeVisible()
        await expect(
          firstTurn.getByRole("link", { name: /^Open dashboard/ })
        ).toBeVisible()
        await expect(
          firstTurn.getByText(dashboardTitle, { exact: true })
        ).toBeVisible()
        await expect(firstTurn.getByText(/^\d+ panels? from /)).toBeVisible()

        const created = await page.request.get(`/api/v1/observe/${firstId}`)
        expect(created.ok()).toBe(true)
        const body = (await created.json()) as {
          request: { dashboard_id: string | null }
        }
        dashboardId = body.request.dashboard_id ?? ""
        expect(dashboardId).not.toBe("")

        await firstTurn.getByText(/^Ran \d+ steps?/).click()
        const steps = firstTurn
          .getByRole("list", { name: "Progress" })
          .getByRole("listitem")
        await expect(steps.first()).toBeVisible()
        await expect(steps.filter({ hasText: /^Saving/ }).first()).toBeVisible()
      })

      await test.step("open the dashboard", async () => {
        await firstTurn.getByRole("link", { name: /^Open dashboard/ }).click()
        await expect(page).toHaveURL(
          new RegExp(`/d/${dashboardId}(/[a-z0-9-]+)?$`)
        )
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(
          dashboardTitle
        )
        await expectPanelsReady(page)
        await expect(
          page.locator("article[data-panel-id]").first()
        ).toBeVisible()
        await expect(card(page)).toBeVisible()
        await page.goBack()
        await expect(firstTurn).toBeVisible()
      })

      await test.step("follow up with a question", async () => {
        const releaseFollowUp = await holdRequestPollsUntilFinished(page)
        const followUpCreated = page.waitForResponse(
          (response) =>
            response.request().method() === "POST" &&
            new URL(response.url()).pathname === "/api/v1/observe"
        )
        await composer(page).fill(followUpQuestion)
        await composer(page).press("Enter")
        const followUp = (await (await followUpCreated).json()) as {
          request: { id: string }
        }

        await expect(questionTurn).toBeVisible()
        await releaseFollowUp(followUp.request.id)
        await expect(
          questionTurn.getByText(`Suggested change: ${followUpPrompt}`)
        ).toBeVisible({ timeout: agentTimeout })
        await expect(page.getByText(/Saved as version 2/)).toHaveCount(0)
        await expect(page).toHaveURL(new RegExp(`/c/${firstId}$`))
      })

      await test.step("revise the dashboard from the answer", async () => {
        const revisionCreated = page.waitForResponse(
          (response) =>
            response.request().method() === "POST" &&
            new URL(response.url()).pathname === "/api/v1/observe"
        )
        await questionTurn
          .getByRole("button", { name: "Revise the dashboard", exact: true })
          .click()
        expect((await revisionCreated).status()).toBe(202)

        const turns = page.getByRole("region", { name: /^Request / })
        await expect(turns.last().getByText(/Saved as version 2/)).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(turns.last().getByText(/^Ran \d+ steps?/)).toBeVisible()
        await expect(page).toHaveURL(new RegExp(`/c/${firstId}$`))
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
        await expect(current).toContainText(`Saved ${refinedDashboardTitle}`)
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
        await expect(page).not.toHaveURL(new RegExp(firstId))
        await expect(page).toHaveURL(requestPath)
        await expect(rows.nth(order.followUp)).toHaveAttribute(
          "aria-current",
          "true"
        )

        await rows.nth(order.current).click()
        await expect(page).toHaveURL(new RegExp(`/c/${firstId}$`))
        await expect(rows.nth(order.current)).toHaveAttribute(
          "aria-current",
          "true"
        )
      })

      await test.step("find it in Recent chats", async () => {
        await page.goto("/ask")
        const recent = page.getByRole("region", { name: "Recent chats" })
        const row = recent
          .getByRole("button", { name: new RegExp(askPrompt) })
          .first()
        await expect(row).toBeVisible()
        await row.click()
        await expect(page).toHaveURL(/\/c\/[0-9a-f-]+$/)
        const turns = page.getByRole("region", { name: /^Request / })
        await expect(turns.first()).toContainText(askPrompt)
        await expect(turns.last()).toContainText(followUpPrompt)
      })

      await test.step("theme", async () => {
        await expect(page.locator("html")).toHaveClass(
          colorScheme === "dark" ? /dark/ : /^(?!.*dark)/
        )
      })
    })
  })
}
