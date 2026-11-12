import { describe, expect, it } from "vitest"

import {
  agentName,
  connectPrompt,
  disconnectLabel,
  disconnectTitle,
  modeChoices,
  modeLabel,
  type ConnectedAgent,
} from "./rows"
import { claudeOAuthSnippet } from "./snippets"

function grant(name: string | null): ConnectedAgent {
  return {
    id: "g1",
    client: { id: "c1", name, redirect_host: "localhost:3118", local: true },
    person: {
      id: "u1",
      name: "Ada Lovelace",
      email: "ada@example.com",
    },
    created_at: "2026-11-11T10:00:00Z",
    last_used_at: null,
    expires_at: "2027-02-09T10:00:00Z",
    state: "active",
    mode: "direct",
    presence: "offline",
    last_seen_at: null,
  }
}

describe("agentName", () => {
  it("shows the registered name or names an unnamed client", () => {
    expect(agentName("Claude Code")).toBe("Claude Code")
    expect(agentName(null)).toBe("Unnamed MCP client")
  })
})

describe("disconnect text", () => {
  it("titles the dialog with the client", () => {
    expect(disconnectTitle("Claude Code")).toBe("Disconnect Claude Code?")
    expect(disconnectTitle(null)).toBe("Disconnect Unnamed MCP client?")
  })

  it("names the person only on the everyone list", () => {
    expect(disconnectLabel(grant("Claude Code"), "mine")).toBe(
      "Disconnect Claude Code"
    )
    expect(disconnectLabel(grant("Claude Code"), "all")).toBe(
      "Disconnect Claude Code of Ada Lovelace"
    )
  })
})

describe("modeLabel", () => {
  it("names both modes", () => {
    expect(modeLabel("direct")).toBe("Direct")
    expect(modeLabel("propose")).toBe("Propose")
  })

  it("offers direct first and explains each choice", () => {
    expect(modeChoices).toEqual([
      {
        value: "direct",
        label: "Direct",
        description: "Saves change dashboards",
      },
      {
        value: "propose",
        label: "Propose",
        description: "Saves become proposals an editor merges",
      },
    ])
  })
})

describe("connectPrompt", () => {
  it("offers the first connection on an empty list and another otherwise", () => {
    expect(connectPrompt(0)).toBe("none")
    expect(connectPrompt(1)).toBe("another")
  })
})

describe("claudeOAuthSnippet", () => {
  it("adds the server over HTTP with no key", () => {
    expect(claudeOAuthSnippet("https://observe.example.com/mcp")).toBe(
      "claude mcp add --transport http slashobserve https://observe.example.com/mcp"
    )
  })
})
