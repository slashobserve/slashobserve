import { describe, expect, it } from "vitest"

import { claudeCodePrompt, commentsListCommand } from "./copy-prompt"

describe("commentsListCommand", () => {
  it("names the dashboard", () => {
    expect(commentsListCommand("abc123")).toBe("observe comments list abc123")
  })
})

describe("claudeCodePrompt", () => {
  it("writes the three lines", () => {
    expect(
      claudeCodePrompt({
        dashboardId: "abc123",
        title: "Checkout health",
        url: "https://observe.example.com/dashboards/abc123",
      })
    ).toBe(
      [
        'Work through the open comments on the SlashObserve dashboard "Checkout health" (https://observe.example.com/dashboards/abc123).',
        "List them with the SlashObserve MCP tool list_comments, or run: observe comments list abc123",
        "For each one, change the dashboard with update_dashboard, reply with reply_to_comment, and resolve it with resolve_comment, passing the version you saved.",
      ].join("\n")
    )
  })

  it("keeps quotes in the title as they are", () => {
    const prompt = claudeCodePrompt({
      dashboardId: "d1",
      title: 'The "checkout" board',
      url: "http://localhost:8080/dashboards/d1",
    })
    expect(prompt.split("\n")[0]).toBe(
      'Work through the open comments on the SlashObserve dashboard "The "checkout" board" (http://localhost:8080/dashboards/d1).'
    )
  })
})
