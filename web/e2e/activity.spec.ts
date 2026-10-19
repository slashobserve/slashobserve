import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { APIRequestContext, Page } from "@playwright/test"

import {
  addVersion,
  createComment,
  createDashboard,
  loadSpec,
  uniqueTitle,
  type Spec,
} from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

const repoRoot = join(import.meta.dirname, "..", "..")
const agentTimeout = 90_000
const rpsTitle = "Checkout requests per second"
const logsTitle = "Log rows by service"
const agentRpsTitle = "Requests per second"
const you = "admin"

function readJson<T>(...segments: string[]): T {
  return JSON.parse(
    readFileSync(join(repoRoot, ...segments), { encoding: "utf8" })
  ) as T
}

function fixturePrompt(file: string): string {
  return readJson<{ prompt: string }>("e2e", "fixtures", "llm", file).prompt
}

const perRoutePrompt = fixturePrompt("comment-per-route.json")
const p99Prompt = fixturePrompt("followup-p99-panel.json")
const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"
const revisionsApi = /\/api\/v1\/dashboards\/[^/]+\/revisions(\?.*)?$/
const commentsPostApi = /\/api\/v1\/dashboards\/[^/]+\/comments$/
const olderVersionNote =
  "You are viewing an older version. Open the latest version to send changes to the agent."
const internalError = JSON.stringify({ error: "boom", code: "internal" })

async function createOverview(request: APIRequestContext) {
  const spec = loadSpec("overview")
  spec.title = uniqueTitle("E2E activity")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

async function createAgentDashboard(request: APIRequestContext) {
  const spec = readJson<{ spec: Spec }>(
    "e2e",
    "fixtures",
    "api",
    "dashboard-valid.json"
  ).spec
  spec.title = uniqueTitle("E2E activity agent")
  const { id } = await createDashboard(request, spec)
  return { id, spec }
}

function card(page: Page) {
  return page.getByRole("complementary", { name: "Activity" })
}

function toggle(page: Page) {
  return page.getByRole("button", { name: /^Activity/ })
}

function entries(page: Page) {
  return card(page)
    .getByRole("list", { name: "Activity" })
    .locator(":scope > li")
}

function entryWith(page: Page, text: string) {
  return entries(page).filter({ hasText: text })
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
  return composerForm(page).getByRole("combobox")
}

function showSelect(page: Page) {
  return card(page).getByRole("combobox", { name: /^Show/ })
}

function scopeSelect(page: Page) {
  return card(page).getByRole("combobox", { name: /^Panel/ })
}

function addCommentButton(page: Page) {
  return composerForm(page).getByRole("button", { name: "Add comment" })
}

function sendToAgentButton(page: Page) {
  return composerForm(page).getByRole("button", { name: "Send to agent" })
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

function dropExpectedErrors(consoleErrors: string[], status: string) {
  for (let index = consoleErrors.length - 1; index >= 0; index--) {
    if (consoleErrors[index].includes(`status of ${status}`)) {
      consoleErrors.splice(index, 1)
    }
  }
}

function clearRejectedRequestLogs(consoleErrors: string[], status: number) {
  const logged = consoleErrors.filter((message) =>
    message.includes(`status of ${status}`)
  )
  expect(logged).not.toHaveLength(0)
  for (const message of logged) {
    consoleErrors.splice(consoleErrors.indexOf(message), 1)
  }
}

async function holdRequestsRunning(page: Page) {
  let held = true
  await page.route(revisionsApi, async (route) => {
    if (route.request().method() !== "GET" || !held) {
      await route.fallback()
      return
    }
    const response = await route.fetch()
    const body = (await response.json()) as {
      revisions: Record<string, unknown>[]
    }
    body.revisions = body.revisions.map((revision) => ({
      ...revision,
      status: "running",
      attempts: 1,
      error: null,
      result_version: null,
      finished_at: null,
    }))
    await route.fulfill({ response, json: body })
  })
  return () => {
    held = false
  }
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("opens by default and remembers being closed", async ({
      page,
      request,
    }) => {
      const { id } = await createOverview(request)
      await openCard(page, id)
      await expect(toggle(page)).toHaveAttribute("aria-pressed", "true")

      await card(page).getByRole("button", { name: "Close panel" }).click()
      await expect(card(page)).toHaveCount(0)
      await expect(toggle(page)).toHaveAttribute("aria-pressed", "false")

      await page.reload()
      await expect(toggle(page)).toHaveAttribute("aria-pressed", "false")
      await expect(card(page)).toHaveCount(0)

      await toggle(page).click()
      await expect(card(page)).toBeVisible()
      await expect(toggle(page)).toHaveAttribute("aria-pressed", "true")
    })

    test("comments on a panel from its menu", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openCard(page, id)
      await expectPanelsReady(page)

      await openPanelMenuItem(page, rpsTitle, "Comment")

      await expect(page).toHaveURL(/panel=rps/)
      await expect(scopeSelect(page).locator("option:checked")).toHaveText(
        rpsTitle
      )
      await expect(targetSelect(page).locator("option:checked")).toHaveText(
        rpsTitle
      )
      await expect(composerBox(page)).toBeFocused()

      await composerBox(page).fill("Latency looks high")
      await addCommentButton(page).click()

      const entry = entryWith(page, "Latency looks high")
      await expect(entry).toHaveCount(1)
      await expect(entry.getByText(you, { exact: true })).toBeVisible()
      await expect(
        entry.getByText("commented on", { exact: true })
      ).toBeVisible()
      await expect(entry.getByRole("button", { name: rpsTitle })).toBeVisible()
      await expect(entry.getByText("Comment", { exact: true })).toBeVisible()
      await expect(composerBox(page)).toHaveValue("")
      await expect(composerBox(page)).toBeFocused()
      await expect(
        page.getByRole("button", { name: "Activity, 1 open comment" })
      ).toBeVisible()
      await expect(
        panel(page, rpsTitle).getByRole("button", {
          name: `1 open comment on ${rpsTitle}`,
        })
      ).toBeVisible()
    })

    test("comments on the whole dashboard", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openCard(page, id)

      await targetSelect(page).selectOption({ label: "The whole dashboard" })
      const box = composerBox(page)
      await box.fill("Overall this looks healthy")

      let release!: () => void
      const held = new Promise<void>((resolve) => (release = resolve))
      await page.route(commentsPostApi, async (route) => {
        if (route.request().method() !== "POST") {
          await route.fallback()
          return
        }
        await held
        await route.fallback()
      })
      await box.press("Control+Enter")

      await expect(box).toHaveAttribute("readonly", "")
      await box.focus()
      await page.keyboard.type(" and more")
      await expect(box).toHaveValue("Overall this looks healthy")
      release()

      const entry = entryWith(page, "Overall this looks healthy")
      await expect(entry).toHaveCount(1)
      await expect(entry.getByText("commented", { exact: true })).toBeVisible()
      await expect(entry.locator("p button")).toHaveCount(0)
      await expect(box).toHaveValue("")
      await expect(
        page.getByRole("button", { name: "Activity, 1 open comment" })
      ).toBeVisible()
    })

    test("replies to a thread", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "Check the dip")
      await openCard(page, id)

      const entry = entryWith(page, "Check the dip")
      const reply = entry.getByRole("button", { name: "Reply", exact: true })
      await reply.click()
      const form = entry.getByRole("form", { name: "Reply to this comment" })
      const box = form.getByRole("textbox", { name: "Reply" })
      await expect(box).toBeFocused()
      await box.fill("Agreed, looking now")
      await box.press("Control+Enter")

      await expect(form).toHaveCount(0)
      const nested = entry.getByRole("list").locator("> li")
      await expect(nested).toHaveCount(1)
      await expect(nested).toContainText("Agreed, looking now")
      await expect(nested.getByText("replied", { exact: true })).toBeVisible()
      await expect(reply).toBeFocused()

      await reply.click()
      await expect(form.getByRole("textbox", { name: "Reply" })).toBeFocused()
      await page.keyboard.press("Escape")
      await expect(form).toHaveCount(0)
      await expect(reply).toBeFocused()
    })

    test("resolves and reopens a thread", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "Looks high")
      await openCard(page, id)
      await expect(
        page.getByRole("button", { name: "Activity, 1 open comment" })
      ).toBeVisible()

      const entry = entryWith(page, "Looks high")
      await entry.getByRole("button", { name: "Resolve", exact: true }).click()

      await expect(entry.getByText(`Resolved by ${you}`)).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Activity", exact: true })
      ).toBeVisible()

      await entry.getByRole("button", { name: "Reopen", exact: true }).click()
      await expect(entry.getByText("Comment", { exact: true })).toBeVisible()
      await expect(entry.getByText(/^Resolved by/)).toHaveCount(0)
      await expect(
        page.getByRole("button", { name: "Activity, 1 open comment" })
      ).toBeVisible()
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

    test("renders comment text as text", async ({ page, request }) => {
      const { id } = await createOverview(request)
      const body = "<b>bold</b> **x**"
      await createComment(request, id, 1, "rps", body)
      await openCard(page, id)

      const entry = entries(page).filter({ hasText: "**x**" })
      await expect(entry).toHaveCount(1)
      await expect(entry.getByText(body, { exact: true })).toBeVisible()
      await expect(entry.locator("b")).toHaveCount(0)
      await expect(entry.locator("strong")).toHaveCount(0)
    })

    test("scopes to a panel and filters", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "Note on requests")
      await createComment(request, id, 1, "error_rate", "Note on errors")
      await openCard(page, id)
      await expect(entries(page)).toHaveCount(3)

      await scopeSelect(page).selectOption({ label: rpsTitle })
      await expect(page).toHaveURL(/panel=rps/)
      await expect(entries(page)).toHaveCount(1)
      await expect(entries(page)).toContainText("Note on requests")

      await scopeSelect(page).selectOption({ label: "All panels" })
      await expect(page).not.toHaveURL(/panel=/)
      await expect(entries(page)).toHaveCount(3)

      await showSelect(page).selectOption({ label: "Changes only" })
      await expect(entries(page)).toHaveCount(1)
      await expect(entries(page)).toContainText("Version 1")
      await expect(entryWith(page, "Note on")).toHaveCount(0)

      await showSelect(page).selectOption({ label: "Open comments" })
      await expect(entries(page)).toHaveCount(2)
      await expect(entryWith(page, "Version 1")).toHaveCount(0)

      await showSelect(page).selectOption({ label: "Changes only" })
      await scopeSelect(page).selectOption({ label: rpsTitle })
      await expect(
        card(page).getByText("No activity matches these filters.")
      ).toBeVisible()
    })

    test("jumps to a panel from the timeline", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "logs", "Too many rows")
      await page.setViewportSize({ width: 1280, height: 360 })
      await openCard(page, id)
      const logs = panel(page, logsTitle)
      await expect(logs).not.toBeInViewport()

      await entryWith(page, "Too many rows")
        .getByRole("button", { name: logsTitle })
        .click()

      await expect(logs).toBeInViewport()
      await expect(logs).toHaveAttribute("data-highlighted", "")
    })

    test("keeps a draft when the card closes", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openCard(page, id)
      await composerBox(page).fill("Not sent yet")

      await card(page).getByRole("button", { name: "Close panel" }).click()
      await expect(card(page)).toHaveCount(0)
      await toggle(page).click()

      await expect(composerBox(page)).toHaveValue("Not sent yet")
    })

    test("shows the limit", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await openCard(page, id)
      const box = composerBox(page)
      const counter = (text: string) => card(page).getByText(text)

      await box.fill("a".repeat(3499))
      await expect(card(page).getByText(/characters? (left|over)/)).toHaveCount(
        0
      )

      await box.fill("a".repeat(3500))
      await expect(counter("500 characters left")).toBeVisible()
      await box.fill("a".repeat(3600))
      await expect(counter("400 characters left")).toBeVisible()
      await expect(addCommentButton(page)).toBeEnabled()

      await box.fill(`  ${"a".repeat(4000)}  `)
      await expect(counter("0 characters left")).toBeVisible()
      await expect(addCommentButton(page)).toBeEnabled()
      await expect(sendToAgentButton(page)).toBeEnabled()

      await box.fill("a".repeat(4001))
      await expect(counter("1 character over")).toBeVisible()
      await expect(addCommentButton(page)).toBeDisabled()
      await expect(sendToAgentButton(page)).toBeDisabled()
    })

    test.describe("with the replay agent", () => {
      test.setTimeout(180_000)

      test("sends a panel request to the agent", async ({ page, request }) => {
        const { id } = await createAgentDashboard(request)
        await openCard(page, id)
        await expectPanelsReady(page)
        const finish = await holdRequestsRunning(page)

        await targetSelect(page).selectOption({ label: agentRpsTitle })
        await composerBox(page).fill(perRoutePrompt)
        await sendToAgentButton(page).click()

        const entry = entryWith(page, perRoutePrompt).filter({
          has: page.getByText("asked the agent on", { exact: true }),
        })
        await expect(entry).toHaveCount(1)
        await expect(
          entry.getByRole("button", { name: /^Requests per second/ })
        ).toBeVisible()
        await expect(entry.getByText("Sent to the agent")).toBeVisible()
        const running = entries(page).filter({
          has: page.getByText("is working on it", { exact: true }),
        })
        await expect(running).toHaveCount(1)

        finish()
        await expect(entry.getByText("Resolved by version 2")).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(running).toHaveCount(0)
        const version = entries(page).filter({
          has: page.getByText("Version 2", { exact: true }),
        })
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
        await thread.getByRole("button", { name: "Reply", exact: true }).click()
        const form = thread.getByRole("form", { name: "Reply to this comment" })
        await form.getByRole("textbox", { name: "Reply" }).fill(p99Prompt)
        await form.getByRole("button", { name: "Send to agent" }).click()

        await expect(form).toHaveCount(0)
        await expect(thread.getByText("Sent to the agent")).toBeVisible()
        await expect(entryWith(page, p99Prompt)).toHaveCount(1)

        finish()
        await expect(thread.getByText("Resolved by version 2")).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(entryWith(page, p99Prompt)).toHaveCount(1)
      })

      test("shows a failed agent request", async ({ page, request }) => {
        const { id } = await createAgentDashboard(request)
        await openCard(page, id)
        const finish = await holdRequestsRunning(page)

        await composerBox(page).fill(unanswerablePrompt)
        await sendToAgentButton(page).click()

        const details = card(page).getByRole("link", {
          name: /Details of the running agent request/,
        })
        await expect(details).toHaveAttribute(
          "href",
          /\/observe\/[0-9a-f-]{36}$/
        )

        finish()
        const entry = entryWith(page, unanswerablePrompt)
        const failure = entry.locator('[data-status="critical"]')
        await expect(failure).toBeVisible({ timeout: agentTimeout })
        await expect(failure).toContainText("The agent could not finish")
        await expect(failure).toContainText("no replay fixture")
      })
    })

    test("explains when Send to agent is unavailable", async ({
      page,
      request,
    }) => {
      const { id, spec } = await createOverview(request)
      await addVersion(request, id, spec, 1, "Second save")
      await openCard(page, id)
      await expect(sendToAgentButton(page)).toBeVisible()

      await versionButton(page).click()
      await page
        .getByRole("menu", { name: "Versions" })
        .getByRole("menuitemradio", { name: /^v1\b/ })
        .click()
      await expect(page).toHaveURL(/version=1/)

      await expect(card(page).getByText(olderVersionNote)).toBeVisible()
      await expect(sendToAgentButton(page)).toHaveCount(0)
      await expect(addCommentButton(page)).toBeVisible()

      await versionButton(page).click()
      await page
        .getByRole("menu", { name: "Versions" })
        .getByRole("menuitemradio", { name: /^v2\b/ })
        .click()
      await expect(page).not.toHaveURL(/version=/)
      await expect(card(page).getByText(olderVersionNote)).toHaveCount(0)

      await page.getByRole("button", { name: /^More/ }).click()
      await page.getByRole("menuitem", { name: "Edit layout" }).click()
      await expect(
        page.getByRole("button", { name: "Save layout" })
      ).toBeVisible()
      await expect(card(page)).toHaveCount(0)
      await expect(toggle(page)).toBeDisabled()
    })

    test("hides Send to agent without a model", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await page.route("**/api/v1/agent", (route) =>
        route.fulfill({
          json: {
            available: false,
            provider: null,
            model: null,
            effort: null,
          },
        })
      )
      await openCard(page, id)

      await expect(
        card(page).getByText("Comments and saved versions.")
      ).toBeVisible()
      await expect(composerBox(page)).toHaveAttribute(
        "placeholder",
        "Leave a comment for the team and Claude Code"
      )
      await composerBox(page).fill("A note for the team")
      await addCommentButton(page).click()

      const entry = entryWith(page, "A note for the team")
      await expect(entry).toHaveCount(1)
      await expect(
        entry.getByText("Open, waiting for Claude Code")
      ).toBeVisible()
      await entry.getByRole("button", { name: "Reply", exact: true }).click()
      await expect(
        entry.getByRole("form", { name: "Reply to this comment" })
      ).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Send to agent" })
      ).toHaveCount(0)
    })

    test("limits queued agent requests", async ({
      page,
      request,
      consoleErrors,
    }) => {
      const { id } = await createOverview(request)
      await page.route(revisionsApi, (route) => {
        if (route.request().method() !== "POST") return route.fallback()
        return route.fulfill({
          status: 429,
          json: { error: "too many revisions", code: "too_many_revisions" },
        })
      })
      await page.route(commentsPostApi, (route) => {
        if (route.request().method() !== "POST") return route.fallback()
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: internalError,
        })
      })
      await openCard(page, id)

      const box = composerBox(page)
      await box.fill("Please split this by route")
      await addCommentButton(page).click()
      await expect(composerForm(page).getByRole("alert")).toHaveText(
        "The comment could not be posted right now. Try again."
      )
      await expect(box).toHaveValue("Please split this by route")
      await expect(box).toBeFocused()
      clearRejectedRequestLogs(consoleErrors, 500)

      await sendToAgentButton(page).click()
      await expect(composerForm(page).getByRole("alert")).toHaveText(
        "This dashboard already has 5 revisions waiting. Try again when one finishes."
      )
      await expect(box).toHaveValue("Please split this by route")
      await expect(box).toBeFocused()
      clearRejectedRequestLogs(consoleErrors, 429)
    })

    test("resizes and remembers the width", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await page.setViewportSize({ width: 1440, height: 900 })
      await openCard(page, id)

      const grip = page.getByRole("separator", { name: "Resize panel" })
      await expect(grip).toHaveAttribute("aria-valuenow", "400")
      await grip.focus()
      for (let press = 0; press < 4; press++) {
        await page.keyboard.press("ArrowLeft")
      }
      await expect(grip).toHaveAttribute("aria-valuenow", "464")
      expect((await card(page).boundingBox())?.width).toBeCloseTo(464, 0)

      await page.reload()
      await expect(
        page.getByRole("separator", { name: "Resize panel" })
      ).toHaveAttribute("aria-valuenow", "464")

      await page.getByRole("separator", { name: "Resize panel" }).focus()
      await page.keyboard.press("End")
      await expect(
        page.getByRole("separator", { name: "Resize panel" })
      ).toHaveAttribute("aria-valuenow", "640")

      await page.getByRole("separator", { name: "Resize panel" }).dblclick()
      await expect(
        page.getByRole("separator", { name: "Resize panel" })
      ).toHaveAttribute("aria-valuenow", "400")
    })

    test("fits a phone", async ({ page, request }) => {
      const { id } = await createOverview(request)
      await page.setViewportSize({ width: 390, height: 844 })
      await openCard(page, id)

      await expect(
        page.getByRole("separator", { name: "Resize panel" })
      ).toHaveCount(0)
      const box = await card(page).boundingBox()
      expect(box?.x ?? 0).toBeGreaterThanOrEqual(0)
      expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(390)
      expect(box?.width ?? 0).toBeGreaterThan(340)
      const below = await page.evaluate(() => {
        const aside = document.querySelector("aside")
        const main = document.querySelector("main")
        if (!aside || !main) return false
        return (
          aside.getBoundingClientRect().top >=
          main.getBoundingClientRect().bottom - 1
        )
      })
      expect(below).toBe(true)
      const overflow = await page.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth
      )
      expect(overflow).toBeLessThanOrEqual(0)
    })

    test("keeps a comment on a pinned version", async ({ page, request }) => {
      const { id, spec } = await createOverview(request)
      await addVersion(request, id, spec, 1, "Second save")
      await openCard(page, id, "?version=1")

      await composerBox(page).fill("Posted from an older version")
      const posted = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          commentsPostApi.test(new URL(response.url()).pathname)
      )
      await addCommentButton(page).click()

      await expect(entryWith(page, "Posted from an older version")).toHaveCount(
        1
      )
      const body = (await (await posted).json()) as {
        comment: { version: number }
      }
      expect(body.comment.version).toBe(2)
    })

    test("opens old links", async ({ page, request }) => {
      const { id } = await createOverview(request)

      await openCard(page, id, "?panel=rps&tab=comments")
      await expect(page).not.toHaveURL(/tab=/)
      await expect(scopeSelect(page)).toHaveValue("rps")
      await expect(panel(page, rpsTitle)).toHaveAttribute(
        "data-highlighted",
        ""
      )

      await page.goto(`/dashboards/${id}?panel=rps&tab=query`)
      const editor = page.getByRole("complementary", {
        name: `Edit ${rpsTitle}`,
      })
      await expect(editor).toBeVisible()
      await expect(editor.locator(".cm-editor")).toBeVisible()
      await expect(page).toHaveURL(/edit=query/)
    })

    test("ignores a second status change while the first is saving", async ({
      page,
      request,
    }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "Looks high")
      let patches = 0
      let release!: () => void
      const held = new Promise<void>((resolve) => (release = resolve))
      let reached!: () => void
      const firstPatch = new Promise<void>((resolve) => (reached = resolve))
      await page.route(
        `**/api/v1/dashboards/${id}/comments/*`,
        async (route) => {
          if (route.request().method() !== "PATCH") {
            await route.fallback()
            return
          }
          if (route.request().url().endsWith("/barrier")) {
            await route.fulfill({ status: 200, json: {} })
            return
          }
          patches++
          reached()
          await held
          await route.continue()
        }
      )
      await openCard(page, id)

      const entry = entryWith(page, "Looks high")
      await entry.getByRole("button", { name: "Resolve", exact: true }).click()
      await firstPatch
      const reopen = entry.getByRole("button", { name: "Reopen", exact: true })
      await expect(reopen).toHaveAttribute("aria-disabled", "true")
      await reopen.focus()
      await page.keyboard.press("Enter")
      await page.evaluate(
        (url) => fetch(url, { method: "PATCH" }),
        `/api/v1/dashboards/${id}/comments/barrier`
      )
      await expect(reopen).toBeFocused()
      expect(patches).toBe(1)

      release()
      await expect(reopen).not.toHaveAttribute("aria-disabled", "true")
      expect(patches).toBe(1)
    })

    test("stops paging when a later page keeps failing", async ({
      page,
      request,
      consoleErrors,
    }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "On the first page")
      let failed = 0
      await page.route(
        `**/api/v1/dashboards/${id}/comments?*`,
        async (route) => {
          const url = new URL(route.request().url())
          if (url.searchParams.has("cursor")) {
            failed++
            await route.fulfill({
              status: 500,
              contentType: "application/json",
              body: internalError,
            })
            return
          }
          const response = await route.fetch()
          const body = (await response.json()) as Record<string, unknown>
          await route.fulfill({
            response,
            json: { ...body, next_cursor: "page-2" },
          })
        }
      )
      await openCard(page, id)
      await expectPanelsReady(page)

      let previous = -1
      await expect
        .poll(
          () => {
            const steady = failed > 0 && failed === previous
            previous = failed
            return steady
          },
          { intervals: [1000], timeout: 15_000 }
        )
        .toBe(true)
      expect(failed).toBeLessThanOrEqual(2)
      dropExpectedErrors(consoleErrors, "500")
    })

    test("shows a comment once when the next page repeats it", async ({
      page,
      request,
    }) => {
      const { id } = await createOverview(request)
      await createComment(request, id, 1, "rps", "Appears on two pages")
      await page.route(
        `**/api/v1/dashboards/${id}/comments?*`,
        async (route) => {
          const url = new URL(route.request().url())
          const paged = url.searchParams.has("cursor")
          url.searchParams.delete("cursor")
          const response = await route.fetch({ url: url.toString() })
          const body = (await response.json()) as Record<string, unknown>
          await route.fulfill({
            response,
            json: { ...body, next_cursor: paged ? null : "page-2" },
          })
        }
      )
      await openCard(page, id)
      await expectPanelsReady(page)

      await expect(entryWith(page, "Appears on two pages")).toHaveCount(1)
    })
  })
}
