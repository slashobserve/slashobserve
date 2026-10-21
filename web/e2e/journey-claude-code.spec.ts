import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { Page } from "@playwright/test"

import { uniqueAccountName, uniqueTitle, type Spec } from "./api"
import { openDashboard, versionButton } from "./dashboards"
import { expect, test } from "./fixtures"
import { mcpKey, mcpTool } from "./mcp"

const repoRoot = join(import.meta.dirname, "..", "..")
const requestText = "Show error rate too"
const replyText = "Added error rate in version 2"

test.use({ permissions: ["clipboard-read", "clipboard-write"] })

function card(page: Page) {
  return page.getByRole("complementary", { name: "Activity" })
}

function entries(page: Page) {
  return card(page)
    .getByRole("list", { name: "Activity" })
    .locator(":scope > li")
}

async function readClipboard(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText())
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })
    test.setTimeout(120_000)

    test("shows a Claude Code dashboard waiting for Claude Code, then resolved over MCP", async ({
      page,
      request,
    }) => {
      const title = uniqueTitle("E2E Claude Code journey")
      const accountName = uniqueAccountName("e2e-claude-code")
      const spec = JSON.parse(
        readFileSync(
          join(repoRoot, "e2e", "fixtures", "api", "dashboard-valid.json"),
          { encoding: "utf8" }
        )
      ).spec as Spec
      spec.title = title
      let id = ""
      let token = ""

      await test.step("setup", async () => {
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
        token = await mcpKey(request, accountName)
        const created = (await mcpTool(request, token, "create_dashboard", {
          spec,
          summary: "Created by Claude Code",
        })) as { dashboard_id: string }
        id = created.dashboard_id
        expect(id).not.toBe("")
      })

      await test.step("breadcrumb", async () => {
        await openDashboard(page, id)
        await expect(
          page.getByRole("navigation", { name: "Breadcrumb" })
        ).toContainText("made by Claude Code")
        await expect(
          card(page).getByText(
            "Comments and versions. Claude Code made this dashboard."
          )
        ).toBeVisible()
      })

      const command = () => `observe comments list ${id}`
      const thread = () => entries(page).filter({ hasText: requestText })

      await test.step("comment waits", async () => {
        const form = card(page).getByRole("form", {
          name: "Add to the activity",
        })
        await form
          .getByRole("textbox", { name: "Comment or request" })
          .fill(requestText)
        await form.getByRole("button", { name: "Add comment" }).click()

        await expect(thread()).toHaveCount(1)
        await expect(
          thread().getByText("Open, waiting for Claude Code")
        ).toBeVisible()
        await expect(
          page.getByRole("button", { name: "Send to agent" })
        ).toHaveCount(0)
        const note = card(page).getByRole("region", {
          name: "Waiting for Claude Code",
        })
        await expect(note.getByText(command())).toBeVisible()
      })

      await test.step("copy", async () => {
        const note = card(page).getByRole("region", {
          name: "Waiting for Claude Code",
        })
        await note.getByRole("button", { name: "Copy prompt" }).click()
        await expect(note.getByRole("button", { name: "Copied" })).toBeVisible()
        const prompt = await readClipboard(page)
        expect(prompt).toContain(command())
        expect(prompt).toContain(title)

        await note.getByRole("button", { name: "Copy command" }).click()
        await expect.poll(() => readClipboard(page)).toBe(command())
      })

      await test.step("Claude Code works the comment", async () => {
        const listed = (await mcpTool(request, token, "list_comments", {
          dashboard_id: id,
        })) as { comments: Array<{ id: string; body: string }> }
        const comment = listed.comments.find((c) => c.body === requestText)
        expect(comment, "the comment is listed over MCP").toBeDefined()

        const saved = (await mcpTool(request, token, "update_dashboard", {
          dashboard_id: id,
          parent_version: 1,
          summary: "Add error rate",
          spec,
        })) as { version: number }
        expect(saved.version).toBe(2)

        await mcpTool(request, token, "reply_to_comment", {
          comment_id: comment!.id,
          body: replyText,
        })
        await mcpTool(request, token, "resolve_comment", {
          comment_id: comment!.id,
          version: 2,
        })
      })

      await test.step("page updates live", async () => {
        const nested = thread().getByRole("list").locator("> li")
        await expect(nested).toHaveCount(1)
        await expect(nested).toContainText(replyText)
        await expect(nested).toContainText(accountName)
        await expect(thread().getByText("Resolved by version 2")).toBeVisible()

        const version = entries(page).filter({
          has: page.getByText("Version 2", { exact: true }),
        })
        await expect(version).toHaveCount(1)
        await expect(
          version.getByText("Claude Code", { exact: true })
        ).toBeVisible()
        await expect(
          version.getByText("saved over MCP", { exact: true })
        ).toBeVisible()
        await expect(versionButton(page)).toHaveText("Live · v2")
      })
    })
  })
}
