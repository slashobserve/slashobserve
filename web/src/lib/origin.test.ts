import { describe, expect, it } from "vitest"

import type { components } from "@/api/schema.gen"

import {
  madeByBreadcrumb,
  madeByChannel,
  isAgentActor,
  madeByDetail,
  versionActor,
} from "./origin"

type Actor = components["schemas"]["Actor"]
type Creator = components["schemas"]["Creator"]

const author = { kind: "user", id: "u1", name: "Ada" } as const

function actor(overrides: Partial<Actor>): Actor {
  return {
    kind: "user",
    id: "u1",
    name: "Ada",
    origin: "web",
    client: null,
    run_id: null,
    label: "Ada",
    ...overrides,
  }
}

function creator(
  actorOverrides: Partial<Actor>,
  overrides: Partial<Creator> = {}
): Creator {
  return {
    author,
    origin: "web",
    origin_client: null,
    via: "direct",
    actor: actor(actorOverrides),
    ...overrides,
  }
}

describe("madeByChannel", () => {
  it.each([
    ["web", "Web"],
    ["cli", "CLI"],
    ["api", "API"],
  ] as const)("names the %s origin", (origin, expected) => {
    expect(madeByChannel(creator({ origin }))).toBe(expected)
  })

  it("has no channel for a dashboard created before origins were recorded", () => {
    expect(madeByChannel(creator({ origin: null }))).toBeNull()
  })

  it("leaves the channel to the label over MCP", () => {
    expect(
      madeByChannel(creator({ origin: "mcp", client: "Claude Code" }))
    ).toBeNull()
  })

  it("leaves the channel to the label when the agent made it", () => {
    expect(madeByChannel(creator({ origin: "web", run_id: "r1" }))).toBeNull()
    expect(madeByChannel(creator({ origin: null, run_id: "r1" }))).toBeNull()
  })
})

describe("madeByDetail", () => {
  const withEmail = { ...author, email: "ada@example.com" }
  const serviceAccount = {
    kind: "service_account",
    id: "k1",
    name: "CI",
  } as const

  it("joins the email and the channel", () => {
    expect(madeByDetail(creator({}, { author: withEmail }))).toBe(
      "ada@example.com · Web"
    )
  })

  it("leaves the channel out over MCP and for the agent", () => {
    expect(
      madeByDetail(
        creator({ origin: "mcp", client: "Claude Code" }, { author: withEmail })
      )
    ).toBe("ada@example.com")
    expect(madeByDetail(creator({ run_id: "r1" }, { author: withEmail }))).toBe(
      "ada@example.com"
    )
  })

  it("shows service account where a service account has no email", () => {
    expect(
      madeByDetail(creator({ origin: "api" }, { author: serviceAccount }))
    ).toBe("service account · API")
  })

  it("leaves out whatever is missing", () => {
    expect(madeByDetail(creator({ origin: null }, { author: withEmail }))).toBe(
      "ada@example.com"
    )
    expect(madeByDetail(creator({ origin: "cli" }))).toBe("CLI")
    expect(
      madeByDetail(creator({}, { author: { ...author, email: "  " } }))
    ).toBe("Web")
    expect(madeByDetail(creator({ origin: null }))).toBe("")
  })
})

describe("madeByBreadcrumb", () => {
  it("names the MCP client the server sent", () => {
    expect(madeByBreadcrumb(creator({ origin: "mcp", client: "Cursor" }))).toBe(
      "made by Cursor"
    )
  })

  it("falls back for an MCP dashboard without a client", () => {
    expect(madeByBreadcrumb(creator({ origin: "mcp" }))).toBe(
      "made by an MCP client"
    )
  })

  it("is null for every other origin", () => {
    expect(madeByBreadcrumb(creator({ origin: "web" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: "cli" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: "api" }))).toBeNull()
    expect(madeByBreadcrumb(creator({ origin: null }))).toBeNull()
  })
})

describe("versionActor", () => {
  it("shows the server's label for a person's save", () => {
    expect(versionActor({ actor: actor({}) })).toEqual({
      who: "Ada",
      verb: "saved",
      agent: false,
    })
  })

  it("marks an MCP save as an agent's", () => {
    expect(
      versionActor({
        actor: actor({
          origin: "mcp",
          client: "Claude Code",
          label: "Ada (via Claude Code)",
        }),
      })
    ).toEqual({ who: "Ada (via Claude Code)", verb: "saved", agent: true })
  })

  it("marks a save by the built-in agent as an agent's", () => {
    expect(
      versionActor({
        actor: actor({ run_id: "r1", label: "Observe Agent for Ada" }),
      })
    ).toEqual({ who: "Observe Agent for Ada", verb: "saved", agent: true })
  })
})

describe("isAgentActor", () => {
  it("is true for the built-in agent and for an MCP client", () => {
    expect(isAgentActor(actor({ run_id: "r1" }))).toBe(true)
    expect(isAgentActor(actor({ origin: "mcp", client: "Claude Code" }))).toBe(
      true
    )
  })

  it("is false for a person on the web, the CLI or the API", () => {
    for (const origin of ["web", "cli", "api", null] as const) {
      expect(isAgentActor(actor({ origin }))).toBe(false)
    }
  })
})
