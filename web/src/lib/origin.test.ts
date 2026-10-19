import { describe, expect, it } from "vitest"

import type { components } from "@/api/schema.gen"

import {
  clientDisplayName,
  madeByBreadcrumb,
  madeByLabel,
  versionActor,
} from "./origin"

type Creator = components["schemas"]["Creator"]

const author = { kind: "user", id: "u1", name: "Ada" } as const

function creator(overrides: Partial<Creator>): Creator {
  return {
    author,
    origin: "web",
    origin_client: null,
    via: "direct",
    ...overrides,
  }
}

describe("clientDisplayName", () => {
  it("maps known clients case-insensitively", () => {
    expect(clientDisplayName("claude-code")).toBe("Claude Code")
    expect(clientDisplayName("Claude Code")).toBe("Claude Code")
    expect(clientDisplayName("claude-ai")).toBe("Claude")
    expect(clientDisplayName("cursor")).toBe("Cursor")
    expect(clientDisplayName("cursor-vscode")).toBe("Cursor")
    expect(clientDisplayName("opencode")).toBe("OpenCode")
    expect(clientDisplayName("codex")).toBe("Codex")
    expect(clientDisplayName("codex-mcp-client")).toBe("Codex")
    expect(clientDisplayName("windsurf")).toBe("Windsurf")
    expect(clientDisplayName("zed")).toBe("Zed")
    expect(clientDisplayName("vscode")).toBe("VS Code")
    expect(clientDisplayName("Visual Studio Code")).toBe("VS Code")
  })

  it("trims before the lookup", () => {
    expect(clientDisplayName("  claude-code  ")).toBe("Claude Code")
  })

  it("returns an unknown client as given, trimmed", () => {
    expect(clientDisplayName("  my-Tool 2 ")).toBe("my-Tool 2")
  })

  it("returns a name that matches an object property as given", () => {
    expect(clientDisplayName("constructor")).toBe("constructor")
    expect(clientDisplayName("Constructor")).toBe("Constructor")
    expect(clientDisplayName("toString")).toBe("toString")
    expect(clientDisplayName(" valueOf ")).toBe("valueOf")
    expect(clientDisplayName("hasOwnProperty")).toBe("hasOwnProperty")
    expect(clientDisplayName("__proto__")).toBe("__proto__")
  })

  it("falls back for an empty or missing name", () => {
    expect(clientDisplayName("")).toBe("MCP client")
    expect(clientDisplayName("   ")).toBe("MCP client")
    expect(clientDisplayName(null)).toBe("MCP client")
    expect(clientDisplayName(undefined)).toBe("MCP client")
  })
})

describe("madeByLabel", () => {
  it("names the requester for an agent-made dashboard", () => {
    expect(madeByLabel(creator({ via: "agent", origin: "mcp" }))).toBe(
      "Agent, for Ada"
    )
  })

  it("names the client for an MCP dashboard", () => {
    expect(
      madeByLabel(creator({ origin: "mcp", origin_client: "claude-code" }))
    ).toBe("Claude Code, Ada")
    expect(madeByLabel(creator({ origin: "mcp" }))).toBe("MCP client, Ada")
  })

  it("keeps an MCP client named like an object property", () => {
    expect(
      madeByLabel(creator({ origin: "mcp", origin_client: "constructor" }))
    ).toBe("constructor, Ada")
    expect(
      madeByLabel(creator({ origin: "mcp", origin_client: "__proto__" }))
    ).toBe("__proto__, Ada")
  })

  it("names the author otherwise", () => {
    expect(madeByLabel(creator({ origin: "web" }))).toBe("Ada")
    expect(madeByLabel(creator({ origin: "cli" }))).toBe("Ada")
    expect(madeByLabel(creator({ origin: "api" }))).toBe("Ada")
    expect(madeByLabel(creator({ origin: null }))).toBe("Ada")
  })
})

describe("madeByBreadcrumb", () => {
  it("names the MCP client", () => {
    expect(
      madeByBreadcrumb(creator({ origin: "mcp", origin_client: "cursor" }))
    ).toBe("made by Cursor")
  })

  it("keeps an MCP client named like an object property", () => {
    expect(
      madeByBreadcrumb(creator({ origin: "mcp", origin_client: "toString" }))
    ).toBe("made by toString")
    expect(
      madeByBreadcrumb(creator({ origin: "mcp", origin_client: "__proto__" }))
    ).toBe("made by __proto__")
  })

  it("falls back for an MCP dashboard without a client", () => {
    expect(madeByBreadcrumb(creator({ origin: "mcp" }))).toBe(
      "made by an MCP client"
    )
    expect(
      madeByBreadcrumb(creator({ origin: "mcp", origin_client: " " }))
    ).toBe("made by an MCP client")
  })

  it("is null for every other origin", () => {
    expect(madeByBreadcrumb(creator({ origin: "web" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: "cli" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: "api" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: null }))).toBeNull()
  })
})

describe("versionActor", () => {
  const base = {
    author,
    origin: "web" as const,
    origin_client: null,
    agent_request_id: null,
  }

  it("attributes an agent save to the agent", () => {
    expect(
      versionActor({
        ...base,
        origin: "mcp",
        origin_client: "claude-code",
        agent_request_id: "r1",
      })
    ).toEqual({ who: "Agent", verb: "saved", agent: true })
  })

  it("attributes an MCP save to the client", () => {
    expect(
      versionActor({ ...base, origin: "mcp", origin_client: "claude-code" })
    ).toEqual({ who: "Claude Code", verb: "saved over MCP", agent: true })
    expect(versionActor({ ...base, origin: "mcp" })).toEqual({
      who: "MCP client",
      verb: "saved over MCP",
      agent: true,
    })
  })

  it("keeps an MCP client named like an object property", () => {
    expect(
      versionActor({ ...base, origin: "mcp", origin_client: "constructor" })
    ).toEqual({ who: "constructor", verb: "saved over MCP", agent: true })
    expect(
      versionActor({ ...base, origin: "mcp", origin_client: "__proto__" })
    ).toEqual({ who: "__proto__", verb: "saved over MCP", agent: true })
  })

  it("attributes any other save to the author", () => {
    expect(versionActor(base)).toEqual({
      who: "Ada",
      verb: "saved",
      agent: false,
    })
    expect(versionActor({ ...base, origin: null })).toEqual({
      who: "Ada",
      verb: "saved",
      agent: false,
    })
  })
})
