import type { Page } from "@playwright/test"

import {
  agentSpec,
  card,
  entries,
  entryWith,
  fixturePrompt,
  holdRequestsRunning,
} from "./activity"
import { createComment, createDashboard, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

const agentTimeout = 90_000
const firstPanelId = "rps"
const firstPanelTitle = "Requests per second"
const secondPanelTitle = "Log rows by service"
const teammateComment = "Is this per pod?"
const ownComment = "Needs a p99"

const p99Question = fixturePrompt("ask-p99.json")
const p99Change = fixturePrompt("followup-p99-panel.json")

function versionEntry(page: Page, version: number) {
  return entries(page).filter({
    has: page.getByText(`Version ${version}`, { exact: true }),
  })
}

function activityToggle(page: Page, openComments: number) {
  return page.getByRole("button", {
    name: `Activity, ${openComments} open comment${openComments === 1 ? "" : "s"}`,
  })
}

test.use({ colorScheme: "light" })
test.setTimeout(180_000)

test("reviews a dashboard, sends a comment to the agent and compares the result", async ({
  page,
  request,
}) => {
  const spec = agentSpec()
  spec.title = uniqueTitle("E2E review journey")
  const { id } = await createDashboard(request, spec)
  const thread = (text: string) => entryWith(page, text)

  await test.step("open", async () => {
    await createComment(request, id, 1, firstPanelId, teammateComment)
    await openDashboard(page, id)
    await expect(card(page)).toBeVisible()
    await expectPanelsReady(page)

    await expect(
      thread(teammateComment).getByText("Open", { exact: true })
    ).toBeVisible()
    await expect(versionEntry(page, 1)).toHaveCount(1)
    await expect(activityToggle(page, 1)).toBeVisible()
  })

  await test.step("comment", async () => {
    await page
      .getByRole("button", { name: `Panel menu: ${secondPanelTitle}` })
      .click()
    await page.getByRole("menuitem", { name: "Comment" }).click()
    const canvasComposer = page.getByRole("dialog", {
      name: "Comment on the canvas",
    })
    await expect(page).not.toHaveURL(/panel=/)
    await canvasComposer
      .getByRole("textbox", { name: "Write a comment" })
      .fill(ownComment)
    await canvasComposer.getByRole("button", { name: "Send" }).click()

    const own = thread(ownComment)
    await expect(own).toHaveCount(1)
    await expect(
      own.getByRole("button", { name: secondPanelTitle })
    ).toBeVisible()
    await expect(activityToggle(page, 2)).toBeVisible()

    await expect(thread(teammateComment)).toHaveCount(1)
    await expect(
      thread(teammateComment).getByRole("button", {
        name: firstPanelTitle,
      })
    ).toBeVisible()
  })

  const finish = await holdRequestsRunning(page)
  const own = thread(ownComment)

  await test.step("send the thread", async () => {
    await own.getByRole("button", { name: "Reply", exact: true }).click()
    const form = card(page).getByRole("form", {
      name: "Add to the activity",
    })
    await form
      .getByRole("textbox", { name: "Comment or request" })
      .fill(`@observe ${p99Question}`)
    await form
      .getByRole("button", { name: "Send and ask Observe Agent" })
      .click()

    const running = own.getByRole("status").filter({
      hasText: "Observe Agent is working on it · View the run",
    })
    await expect(running).toHaveCount(1)
    await expect(
      running.getByRole("link", { name: "View the run" })
    ).toHaveAttribute("href", /\/c\/[0-9a-f-]{36}$/)
  })

  await test.step("answered", async () => {
    finish()
    await expect(own.getByText(`Suggested change: ${p99Change}`)).toBeVisible({
      timeout: agentTimeout,
    })
    await expect(versionEntry(page, 2)).toHaveCount(0)
    await expect(versionButton(page)).toHaveText("Live · v1")
  })

  await test.step("revised", async () => {
    await own
      .getByRole("button", { name: "Revise the dashboard", exact: true })
      .click()
    await expect(
      own.getByText(`Revise the dashboard: ${p99Change}`)
    ).toBeVisible()
    await expect(
      own.getByText("Saved version 2 of this dashboard.")
    ).toBeVisible({ timeout: agentTimeout })
    await expect(versionEntry(page, 2)).toHaveCount(1)
    await expect(versionButton(page)).toHaveText("Live · v2")
    await expect(activityToggle(page, 2)).toBeVisible()
    await expect(
      thread(teammateComment).getByText("Open", { exact: true })
    ).toBeVisible()
  })

  await test.step("compare", async () => {
    await versionEntry(page, 2)
      .getByRole("link", { name: /See changes/ })
      .click()

    await expect(page).toHaveURL(
      new RegExp(`/d/${id}/versions\\?.*from=1&to=2`)
    )
    await expect(
      page.getByRole("heading", { level: 1, name: /^Versions of / })
    ).toBeVisible()
    await expect(
      page.getByRole("heading", {
        level: 2,
        name: "Changes from v1 to v2",
      })
    ).toBeVisible()
    await expect(
      page.getByRole("region", { name: 'Panel "p99 latency"' })
    ).toBeVisible()

    await page.getByRole("link", { name: "Back to dashboard" }).click()
    await expect(page).toHaveURL(new RegExp(`/d/${id}(/[a-z0-9-]+)?(\\?|$)`))
    await expect(card(page)).toBeVisible()
    await expect(panel(page, firstPanelTitle)).toBeVisible()
    await expect(panel(page, "p99 latency")).toBeVisible()
  })
})
