import type { APIRequestContext, Locator, Page } from "@playwright/test"

import {
  agentSpec,
  card,
  entries,
  entryWith,
  fixturePrompt,
  holdRequestsRunning,
} from "./activity"
import { createComment, createDashboard, loadSpec, uniqueTitle } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

const agentTimeout = 90_000
const rpsTitle = "Shop requests per second"
const agentRpsTitle = "Requests per second"
const you = "admin"

const perRouteQuestion = fixturePrompt("ask-per-route.json")
const perRouteChange = fixturePrompt("comment-per-route.json")
const p99Question = fixturePrompt("ask-p99.json")
const p99Change = fixturePrompt("followup-p99-panel.json")
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E activity")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

async function createAgentDashboard(request: APIRequestContext) {
  const spec = agentSpec()
  spec.title = uniqueTitle("E2E activity agent")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function composerForm(page: Page) {
  return card(page).getByRole("form", { name: "Add to the activity" })
}

function composerBox(page: Page) {
  return composerForm(page).getByRole("textbox", {
    name: "Comment or request",
  })
}

function targetSelect(page: Page) {
  return composerForm(page).getByRole("combobox", { name: "On", exact: true })
}

async function pickOption(page: Page, select: Locator, label: string) {
  await select.click()
  await page.getByRole("option", { name: label, exact: true }).click()
}

function askAgentButton(page: Page) {
  return composerForm(page).getByRole("button", { name: /^Send and ask / })
}

async function openCard(page: Page, id: string, search = "") {
  await openDashboard(page, id, search)
  await expect(card(page)).toBeVisible()
}

async function openPanelMenuItem(page: Page, title: string, item: string) {
  await page.getByRole("button", { name: `Panel menu: ${title}` }).click()
  await page.getByRole("menuitem", { name: item }).click()
}

async function waitForLive(page: Page) {
  await expect(versionButton(page)).toHaveText(/^Live · v\d+$/)
}

test.use({ colorScheme: "light" })

test("comments on a panel from its menu", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openCard(page, id)
  await expectPanelsReady(page)

  await openPanelMenuItem(page, rpsTitle, "Comment")

  const canvasComposer = page.getByRole("dialog", {
    name: "Comment on the canvas",
  })
  await expect(page).not.toHaveURL(/panel=/)
  await expect(canvasComposer.getByText(rpsTitle)).toBeVisible()

  await canvasComposer
    .getByRole("textbox", { name: "Write a comment" })
    .fill("Latency looks high")
  await canvasComposer.getByRole("button", { name: "Send" }).click()
  await expect(canvasComposer).toHaveCount(0)

  const entry = entryWith(page, "Latency looks high")
  await expect(entry).toHaveCount(1)
  await expect(entry.getByText(you, { exact: true })).toBeVisible()
  await expect(entry.getByText("on", { exact: true })).toBeVisible()
  await expect(entry.getByText("commented on", { exact: true })).toHaveCount(0)
  await expect(entry.getByRole("button", { name: rpsTitle })).toBeVisible()
  await expect(entry.getByText("Open", { exact: true })).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Activity, 1 open comment" })
  ).toBeVisible()
  await expect(
    panel(page, rpsTitle).getByRole("button", {
      name: `1 open comment on ${rpsTitle}`,
    })
  ).toBeVisible()
})

test("replies to a thread", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await createComment(request, id, 1, "rps", "Check the dip")
  await openCard(page, id)

  const entry = entryWith(page, "Check the dip")
  await entry.getByRole("button", { name: "Reply", exact: true }).click()
  const replying = composerForm(page).getByText(/^Replying in /)
  await expect(replying).toBeVisible()
  await expect(composerBox(page)).toBeFocused()
  await composerBox(page).fill("Agreed, looking now")
  await composerBox(page).press("Control+Enter")

  const nested = entry.getByRole("list").locator("> li")
  await expect(nested).toHaveCount(1)
  await expect(nested).toContainText("Agreed, looking now")
  await expect(nested.getByText("replied", { exact: true })).toHaveCount(0)
  await expect(composerBox(page)).toHaveValue("")
  await expect(replying).toBeVisible()

  await composerForm(page)
    .getByRole("button", { name: /^Remove Replying in / })
    .click()
  await expect(replying).toHaveCount(0)
  await expect(composerBox(page)).toBeFocused()
})

test("shows a teammate's comment live", async ({ page, request }) => {
  const { id } = await createOverview(request)
  await openCard(page, id)
  await expectPanelsReady(page)
  await waitForLive(page)
  await composerBox(page).fill("Still typing")

  await createComment(request, id, 1, "rps", "From the API")

  await expect(entryWith(page, "From the API")).toHaveCount(1, {
    timeout: 10_000,
  })
  await expect(composerBox(page)).toHaveValue("Still typing")
})

test.describe("with the replay agent", () => {
  test.setTimeout(180_000)

  test("sends a panel request to the agent", async ({ page, request }) => {
    const { id } = await createAgentDashboard(request)
    await openCard(page, id)
    await expectPanelsReady(page)
    const finish = await holdRequestsRunning(page)

    await pickOption(page, targetSelect(page), agentRpsTitle)
    await composerBox(page).fill(`@observe ${perRouteQuestion}`)
    await askAgentButton(page).click()

    const entry = entryWith(page, perRouteQuestion).filter({
      has: page.getByText("on", { exact: true }),
    })
    await expect(entry).toHaveCount(1)
    await expect(
      entry.getByRole("button", { name: /^Requests per second/ })
    ).toBeVisible()
    const running = entry.getByRole("status").filter({
      hasText: "Observe Agent is working on it · View the run",
    })
    await expect(running).toHaveCount(1)

    finish()
    await expect(
      entry.getByText(`Suggested change: ${perRouteChange}`)
    ).toBeVisible({ timeout: agentTimeout })
    await expect(running).toHaveCount(0)
    const version = entries(page).filter({
      has: page.getByText("Version 2", { exact: true }),
    })
    await expect(version).toHaveCount(0)
    await expect(versionButton(page)).toHaveText("Live · v1")

    await entry
      .getByRole("button", { name: "Revise the dashboard", exact: true })
      .click()
    await expect(
      entry.getByRole("button", { name: "Revision asked for" })
    ).toBeDisabled()
    await expect(
      entry.getByText(`Revise the dashboard: ${perRouteChange}`)
    ).toBeVisible()
    await expect(
      entry.getByText("Saved version 2 of this dashboard.")
    ).toBeVisible({ timeout: agentTimeout })
    await expect(running).toHaveCount(0, { timeout: agentTimeout })
    await expect(version).toHaveCount(1)
    await expect(
      version.getByRole("link", { name: /See changes/ })
    ).toHaveAttribute("href", /versions\?from=1&to=2/)
  })

  test("sends a thread to the agent and resolves it with the version", async ({
    page,
    request,
  }) => {
    const { id } = await createAgentDashboard(request)
    await createComment(request, id, 1, "rps", "Can we see p99 too?")
    await openCard(page, id)
    const finish = await holdRequestsRunning(page)

    const thread = entryWith(page, "Can we see p99 too?")
    const promptEntries = entryWith(page, p99Change).filter({
      hasNot: page.getByText("Version 2", { exact: true }),
    })
    await thread.getByRole("button", { name: "Reply", exact: true }).click()
    await composerBox(page).fill(`@observe ${p99Question}`)
    await composerForm(page)
      .getByRole("button", { name: "Send and ask Observe Agent" })
      .click()

    await expect(
      thread.getByText(/Observe Agent is working on it/)
    ).toBeVisible()

    finish()
    await expect(
      thread.getByText(`Suggested change: ${p99Change}`)
    ).toBeVisible({ timeout: agentTimeout })
    await expect(versionButton(page)).toHaveText("Live · v1")

    await thread
      .getByRole("button", { name: "Revise the dashboard", exact: true })
      .click()
    await expect(
      thread.getByText(`Revise the dashboard: ${p99Change}`)
    ).toBeVisible()
    await expect(
      thread.getByText("Saved version 2 of this dashboard.")
    ).toBeVisible({ timeout: agentTimeout })
    await expect(promptEntries).toHaveCount(1)
    await expect(
      entryWith(page, 'Added "p99 latency"').filter({
        has: page.getByText("Version 2", { exact: true }),
      })
    ).toHaveCount(1)
  })

  test("shows a failed agent request", async ({ page, request }) => {
    const { id } = await createAgentDashboard(request)
    await openCard(page, id)

    await composerBox(page).fill(`@observe ${unanswerablePrompt}`)
    await askAgentButton(page).click()

    const entry = entryWith(page, unanswerablePrompt)
    await expect(entry).toHaveCount(1)
    const failure = entry.getByText(/^The run failed: /)
    await expect(failure).toBeVisible({ timeout: agentTimeout })
    await expect(failure).toContainText("no replay fixture")
    await expect(
      entry.getByRole("link", { name: "View the run" })
    ).toHaveAttribute("href", /\/c\/[0-9a-f-]{36}$/)
  })
})
