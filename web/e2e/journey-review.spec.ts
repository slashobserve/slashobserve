import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { Page } from "@playwright/test"

import { createComment, createDashboard, uniqueTitle, type Spec } from "./api"
import {
  expectPanelsReady,
  openDashboard,
  panel,
  versionButton,
} from "./dashboards"
import { expect, test } from "./fixtures"

const repoRoot = join(import.meta.dirname, "..", "..")
const agentTimeout = 90_000
const firstPanelId = "rps"
const firstPanelTitle = "Requests per second"
const secondPanelTitle = "Log rows by service"
const teammateComment = "Is this per pod?"
const ownComment = "Needs a p99"
const revisionsApi = /\/api\/v1\/dashboards\/[^/]+\/revisions(\?.*)?$/

function readJson<T>(...segments: string[]): T {
  return JSON.parse(
    readFileSync(join(repoRoot, ...segments), { encoding: "utf8" })
  ) as T
}

const p99Prompt = readJson<{ prompt: string }>(
  "e2e",
  "fixtures",
  "llm",
  "followup-p99-panel.json"
).prompt

function card(page: Page) {
  return page.getByRole("complementary", { name: "Activity" })
}

function entries(page: Page) {
  return card(page)
    .getByRole("list", { name: "Activity" })
    .locator(":scope > li")
}

function entryWith(page: Page, text: string) {
  return entries(page).filter({ hasText: text })
}

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
    test.setTimeout(180_000)

    test("reviews a dashboard, sends a comment to the agent and compares the result", async ({
      page,
      request,
    }) => {
      const spec = readJson<{ spec: Spec }>(
        "e2e",
        "fixtures",
        "api",
        "dashboard-valid.json"
      ).spec
      spec.title = uniqueTitle("E2E review journey")
      const { id } = await createDashboard(request, spec)
      const thread = (text: string) => entryWith(page, text)

      await test.step("open", async () => {
        await createComment(request, id, 1, firstPanelId, teammateComment)
        await openDashboard(page, id)
        await expect(card(page)).toBeVisible()
        await expectPanelsReady(page)

        await expect(
          thread(teammateComment).getByText("Comment", { exact: true })
        ).toBeVisible()
        await expect(versionEntry(page, 1)).toHaveCount(1)
        await expect(activityToggle(page, 1)).toBeVisible()
      })

      await test.step("comment", async () => {
        await page
          .getByRole("button", { name: `Panel menu: ${secondPanelTitle}` })
          .click()
        await page.getByRole("menuitem", { name: "Comment" }).click()
        await expect(page).toHaveURL(/panel=log_services/)

        const form = card(page).getByRole("form", {
          name: "Add to the activity",
        })
        await form
          .getByRole("textbox", { name: "Comment or request" })
          .fill(ownComment)
        await form.getByRole("button", { name: "Add comment" }).click()

        const own = thread(ownComment)
        await expect(own).toHaveCount(1)
        await expect(
          own.getByRole("button", { name: secondPanelTitle })
        ).toBeVisible()
        await expect(activityToggle(page, 2)).toBeVisible()

        await card(page)
          .getByRole("combobox", { name: /^Panel/ })
          .selectOption({ label: "All panels" })
        await expect(page).not.toHaveURL(/panel=/)
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
        const form = own.getByRole("form", { name: "Reply to this comment" })
        await form.getByRole("textbox", { name: "Reply" }).fill(p99Prompt)
        await form.getByRole("button", { name: "Send to agent" }).click()

        await expect(form).toHaveCount(0)
        await expect(own.getByText("Sent to the agent")).toBeVisible()
        const running = entries(page).filter({
          has: page.getByText("is working on it", { exact: true }),
        })
        await expect(running).toHaveCount(1)
        await expect(
          running.getByRole("link", { name: /Details of the running agent/ })
        ).toBeVisible()
      })

      await test.step("resolved", async () => {
        finish()
        await expect(own.getByText("Resolved by version 2")).toBeVisible({
          timeout: agentTimeout,
        })
        await expect(versionEntry(page, 2)).toHaveCount(1)
        await expect(versionButton(page)).toHaveText("Live · v2")
        await expect(activityToggle(page, 1)).toBeVisible()
        await expect(
          thread(teammateComment).getByText("Comment", { exact: true })
        ).toBeVisible()
      })

      await test.step("compare", async () => {
        await versionEntry(page, 2)
          .getByRole("link", { name: /See changes/ })
          .click()

        await expect(page).toHaveURL(
          new RegExp(`/dashboards/${id}/versions\\?.*from=1&to=2`)
        )
        await expect(
          page.getByRole("heading", { level: 1, name: /: history$/ })
        ).toBeVisible()

        await page.getByRole("link", { name: "Back to dashboard" }).click()
        await expect(page).toHaveURL(new RegExp(`/dashboards/${id}(\\?|$)`))
        await expect(card(page)).toBeVisible()
        await expect(panel(page, firstPanelTitle + " by route")).toBeVisible()
      })
    })
  })
}
