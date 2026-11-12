import { describe, expect, it } from "vitest"

import {
  agentMentionsIn,
  agentToken,
  insertMention,
  mentionedAgents,
  mentionName,
  mentionOptions,
  mentionSegments,
  mentionToken,
  mentionTriggerAt,
  observeAgent,
  threadParticipants,
} from "./mentions"

const adaId = "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f"
const adaToken = `@[Ada](user:${adaId})`

describe("mentionSegments", () => {
  it("returns one text segment for a body without mentions", () => {
    expect(mentionSegments("plain text")).toEqual([
      { kind: "text", text: "plain text" },
    ])
    expect(mentionSegments("")).toEqual([])
  })

  it("splits a user token and the agent handle out of the text", () => {
    expect(mentionSegments(`hi ${adaToken}, ask @observe now`)).toEqual([
      { kind: "text", text: "hi " },
      {
        kind: "mention",
        ref: { kind: "user", id: adaId },
        name: "Ada",
        raw: adaToken,
      },
      { kind: "text", text: ", ask " },
      {
        kind: "mention",
        ref: { kind: "agent", id: "observe" },
        name: "observe",
        raw: "@observe",
      },
      { kind: "text", text: " now" },
    ])
  })

  it.each([
    ["a longer handle", "@observers"],
    ["an email address", "email@observe.io"],
    ["a hyphenated handle", "@observe-bot"],
    ["a different case", "@Observe"],
    ["a token without a uuid", "@[Ada](user:not-a-uuid)"],
    ["a token with a newline in the name", `@[A\nda](user:${adaId})`],
    ["a token without the user scheme", `@[Ada](${adaId})`],
  ])("leaves %s as text", (_name, body) => {
    expect(mentionSegments(body)).toEqual([{ kind: "text", text: body }])
  })

  it("accepts the agent handle next to punctuation", () => {
    const kinds = mentionSegments("(@observe), @observe. @observe!").map(
      (segment) => segment.kind
    )
    expect(kinds.filter((kind) => kind === "mention")).toHaveLength(3)
  })

  it("does not read the agent handle inside a user token's name", () => {
    const body = `@[@observe](user:${adaId})`
    const segments = mentionSegments(body)
    expect(segments).toHaveLength(1)
    expect(segments[0]).toMatchObject({ ref: { kind: "user", id: adaId } })
  })
})

describe("mentionToken", () => {
  it("writes the token the server parses", () => {
    expect(mentionToken({ id: adaId, name: "Ada" })).toBe(adaToken)
  })

  it("drops brackets and newlines from the name", () => {
    expect(mentionToken({ id: adaId, name: "A[d]a\n" })).toBe(
      `@[Ada](user:${adaId})`
    )
    expect(mentionToken({ id: adaId, name: "[]" })).toBe(
      `@[user](user:${adaId})`
    )
  })
})

describe("insertMention", () => {
  it.each([
    ["at the start", "hello", 0, 0, "@observe hello", 9],
    ["after a space", "hi there", 3, 3, "hi @observe there", 12],
    ["glued to a word", "hi", 2, 2, "hi @observe ", 12],
    ["over a selection", "hi XXX there", 3, 6, "hi @observe there", 11],
    ["at the end after a newline", "hi\n", 3, 3, "hi\n@observe ", 12],
  ])("inserts %s", (_name, value, start, end, expected, caret) => {
    expect(insertMention(value, start, end, "@observe")).toEqual({
      value: expected,
      caret,
    })
  })
})

describe("threadParticipants", () => {
  it("lists each user once, in first-seen order, and skips service accounts", () => {
    const people = threadParticipants([
      { kind: "user", id: "a", name: "Ada" },
      { kind: "service_account", id: "s", name: "Bot" },
      { kind: "user", id: "g", name: "Grace" },
      { kind: "user", id: "a", name: "Ada" },
    ])
    expect(people).toEqual([
      { id: "a", name: "Ada" },
      { id: "g", name: "Grace" },
    ])
  })
})

describe("mentionName", () => {
  const recorded = [
    { kind: "user" as const, id: adaId, name: "Ada Lovelace" },
    { kind: "agent" as const, id: "observe", name: "observe" },
  ]

  it("uses the name the server recorded, not the one typed in the token", () => {
    const [segment] = mentionSegments(`@[Admin](user:${adaId})`)
    if (segment.kind !== "mention") throw new Error("expected a mention")
    expect(mentionName(segment, recorded)).toBe("Ada Lovelace")
  })

  it("returns null for a token the server did not record", () => {
    const [segment] = mentionSegments(
      "@[Ghost](user:00000000-0000-0000-0000-000000000000)"
    )
    if (segment.kind !== "mention") throw new Error("expected a mention")
    expect(mentionName(segment, recorded)).toBeNull()
  })
})

describe("mentionTriggerAt", () => {
  it.each([
    ["hi @", 4, 3],
    ["@", 1, 0],
    ["line\n@", 6, 5],
    ["a@", 2, null],
    ["hi @x", 5, null],
    ["hi ", 3, null],
    ["", 0, null],
  ])("for %j with the caret at %i returns %s", (value, caret, expected) => {
    expect(mentionTriggerAt(value, caret)).toBe(expected)
  })
})

const grantId = "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d"
const accountId = "b1f0e2d3-4c5b-4a6d-9e7f-8a9b0c1d2e3f"
const claude = { kind: "grant" as const, id: grantId, name: "Claude Code" }
const deployBot = {
  kind: "service_account" as const,
  id: accountId,
  name: "Deploy bot",
}

describe("agentToken", () => {
  it.each([
    ["the built-in agent", observeAgent, "@observe"],
    ["a connected agent", claude, `@[Claude Code](grant:${grantId})`],
    [
      "a service account",
      deployBot,
      `@[Deploy bot](service_account:${accountId})`,
    ],
    [
      "a name with brackets",
      { ...claude, name: "[Claude]\n" },
      `@[Claude](grant:${grantId})`,
    ],
  ])("writes the token the server parses for %s", (_name, agent, token) => {
    expect(agentToken(agent)).toBe(token)
  })
})

describe("agentMentionsIn", () => {
  it.each([
    ["no mention", "plain text", 0],
    ["only a person", `hi ${adaToken}`, 0],
    ["the built-in agent", "@observe look", 1],
    ["a connected agent", `${agentToken(claude)} look`, 1],
    ["the same agent twice", `@observe and @observe again`, 1],
    ["the same grant twice", `${agentToken(claude)} ${agentToken(claude)}`, 1],
    ["two different agents", `@observe ${agentToken(deployBot)}`, 2],
    [
      "three kinds with a person",
      `@observe ${agentToken(claude)} ${agentToken(deployBot)} ${adaToken}`,
      3,
    ],
  ])("counts %s", (_name, body, count) => {
    expect(agentMentionsIn(body)).toBe(count)
  })
})

describe("mentionedAgents", () => {
  it("names the built-in agent and each connected agent once", () => {
    expect(
      mentionedAgents(`@observe ${agentToken(claude)} @observe`).map(
        (agent) => agent.name
      )
    ).toEqual(["Observe Agent", "Claude Code"])
  })
})

describe("mentionSegments for agent tokens", () => {
  it("reads a grant and a service account token", () => {
    const segments = mentionSegments(
      `${agentToken(claude)} ${agentToken(deployBot)}`
    )
    expect(segments.map((segment) => segment.kind)).toEqual([
      "mention",
      "text",
      "mention",
    ])
    expect(segments[0]).toMatchObject({
      ref: { kind: "grant", id: grantId },
      name: "Claude Code",
    })
    expect(segments[2]).toMatchObject({
      ref: { kind: "service_account", id: accountId },
    })
  })
})

describe("mentionOptions", () => {
  it("offers the Observe Agent first when no one else is given", () => {
    expect(mentionOptions([], [])).toEqual([
      {
        value: "agent:observe",
        label: "Observe Agent",
        token: "@observe",
        agent: true,
        hint: "@observe",
      },
    ])
  })

  it("lists the Observe Agent once, then other agents, then people", () => {
    const options = mentionOptions(
      [
        { id: adaId, name: "Ada" },
        { id: "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c3e", name: "" },
      ],
      [observeAgent, claude, deployBot]
    )
    expect(options.map((option) => option.value)).toEqual([
      "agent:observe",
      `grant:${grantId}`,
      `service_account:${accountId}`,
      `user:${adaId}`,
      "user:3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c3e",
    ])
    expect(options[1]).toMatchObject({
      label: "Claude Code",
      token: `@[Claude Code](grant:${grantId})`,
      agent: true,
    })
    expect(options[3]).toMatchObject({ token: adaToken, hint: null })
    expect(options[4].label).toBe("Unnamed")
  })
})
