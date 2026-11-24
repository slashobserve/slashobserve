import { describe, expect, it } from "vitest"

import {
  addPick,
  agentMentionsIn,
  agentToken,
  inlineKeyStep,
  inlineMentionStep,
  insertMention,
  mentionedAgents,
  mentionName,
  mentionOptions,
  mentionSegments,
  mentionToken,
  mentionTriggerAt,
  observeAgent,
  threadParticipants,
  toDisplay,
  toWire,
  typedMentionEnd,
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

describe("typedMentionEnd", () => {
  it.each([
    ["only the @ is there", "hi @", 3, 4, 4],
    ["text was typed after the @", "hi @obs", 3, 7, 7],
    ["the caret is still right after the @", "@foo", 0, 1, 1],
    ["the caret is before the @", "hi @obs", 3, 0, 4],
    ["the typed run has a space", "hi @ob s", 3, 8, 4],
    ["a hyphen and a digit belong to the run", "@my-bot2", 0, 8, 8],
  ])("%s", (_name, value, trigger, caret, expected) => {
    expect(typedMentionEnd(value, trigger, caret)).toBe(expected)
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
  it("offers no agent when none is given, as with no model configured", () => {
    expect(mentionOptions([], [])).toEqual([])
  })

  it("offers the Observe Agent when it is given", () => {
    expect(mentionOptions([], [observeAgent])).toEqual([
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

describe("mention picks", () => {
  const graceId = "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d"
  const graceToken = `@[Grace](user:${graceId})`

  it("shows a mention as @Name and turns it back into the token", () => {
    const added = addPick([], "Ada", adaToken)
    expect(added.display).toBe("@Ada")
    expect(toWire("ping @Ada now", added.picks)).toBe(`ping ${adaToken} now`)
  })

  it("keeps a pick once and numbers a second person with the same name", () => {
    const first = addPick([], "Ada", adaToken)
    expect(addPick(first.picks, "Ada", adaToken).display).toBe("@Ada")
    const second = addPick(first.picks, "Ada", graceToken)
    expect(second.display).toBe("@Ada 2")
    expect(toWire("@Ada and @Ada 2", second.picks)).toBe(
      `${adaToken} and ${graceToken}`
    )
  })

  it("leaves a typed name alone when it was not picked or is part of a word", () => {
    const { picks } = addPick([], "Ada", adaToken)
    expect(toWire("@Grace and mail@Ada and @Adam", picks)).toBe(
      "@Grace and mail@Ada and @Adam"
    )
  })

  it("rebuilds the display text and the picks from a stored body", () => {
    const shown = toDisplay(`hi ${adaToken}, ask @observe and ${graceToken}`)
    expect(shown.text).toBe("hi @Ada, ask @observe and @Grace")
    expect(toWire(shown.text, shown.picks)).toBe(
      `hi ${adaToken}, ask @observe and ${graceToken}`
    )
  })

  it("escapes names that hold pattern characters", () => {
    const { picks } = addPick([], "A.(b)+", `@[A.(b)+](user:${adaId})`)
    expect(toWire("hi @A.(b)+ there", picks)).toBe(
      `hi @[A.(b)+](user:${adaId}) there`
    )
  })
})

function typeThrough(text: string, matching: (query: string) => number) {
  let value = ""
  let step: ReturnType<typeof inlineMentionStep> = { open: false }
  let trigger: number | null = null
  for (const character of text) {
    value += character
    if (trigger === null) trigger = mentionTriggerAt(value, value.length)
    if (trigger === null) continue
    const query = value.slice(trigger + 1)
    step = inlineMentionStep({
      value,
      trigger,
      caret: value.length,
      matching: matching(query.split(/\s/)[0]),
      searching: false,
    })
    if (!step.open) return { value, step, closedAt: value.length }
  }
  return { value, step, closedAt: null }
}

const knownHandles = (query: string) =>
  "observe agent".includes(query.toLowerCase()) ? 1 : 0

describe("inlineMentionStep", () => {
  it("stays open while the handle is typed and closes at the space after @observe", () => {
    const typed = typeThrough("@observe add a panel", knownHandles)
    expect(typed.closedAt).toBe("@observe ".length)
    expect(typed.value).toBe("@observe ")
  })

  it("closes at the space after an unknown handle even while it still matched", () => {
    expect(
      inlineMentionStep({
        value: "@nobody ",
        trigger: 0,
        caret: 8,
        matching: 1,
        searching: false,
      })
    ).toEqual({ open: false })
  })

  it("closes at once when nobody matches the typed word", () => {
    const typed = typeThrough("@zzz", knownHandles)
    expect(typed.closedAt).toBe("@z".length)
  })

  it("waits for the people search before giving up on an empty match", () => {
    expect(
      inlineMentionStep({
        value: "@ada.l",
        trigger: 0,
        caret: 6,
        matching: 0,
        searching: true,
      })
    ).toEqual({ open: true, query: "ada.l" })
  })

  it("keeps an email address typed after the @ as one query", () => {
    expect(
      inlineMentionStep({
        value: "hi @ada.lab@example.com",
        trigger: 3,
        caret: 23,
        matching: 1,
        searching: false,
      })
    ).toEqual({ open: true, query: "ada.lab@example.com" })
  })

  it("never opens for an email address typed mid-sentence", () => {
    const typed = typeThrough("write to a@b.com now", knownHandles)
    expect(typed.step).toEqual({ open: false })
    expect(typed.closedAt).toBeNull()
  })

  it("closes when the @ is deleted or the caret moves before it", () => {
    const base = { trigger: 3, matching: 1, searching: false }
    expect(inlineMentionStep({ ...base, value: "hi ", caret: 3 })).toEqual({
      open: false,
    })
    expect(inlineMentionStep({ ...base, value: "hi @a", caret: 2 })).toEqual({
      open: false,
    })
  })

  it("is open with an empty query right after the @", () => {
    expect(
      inlineMentionStep({
        value: "hi @",
        trigger: 3,
        caret: 4,
        matching: 3,
        searching: false,
      })
    ).toEqual({ open: true, query: "" })
  })
})

describe("inlineKeyStep", () => {
  it("picks the highlighted match on Enter", () => {
    expect(inlineKeyStep({ key: "Enter", active: 1, count: 3 })).toEqual({
      kind: "pick",
      index: 1,
    })
  })

  it("holds Enter while matches are offered and none is highlighted", () => {
    expect(inlineKeyStep({ key: "Enter", active: -1, count: 3 })).toEqual({
      kind: "hold",
    })
  })

  it("lets Enter through when nothing is offered", () => {
    expect(inlineKeyStep({ key: "Enter", active: 0, count: 0 })).toEqual({
      kind: "pass",
    })
  })

  it("moves through the matches and wraps", () => {
    expect(inlineKeyStep({ key: "ArrowDown", active: -1, count: 3 })).toEqual({
      kind: "move",
      index: 0,
    })
    expect(inlineKeyStep({ key: "ArrowDown", active: 2, count: 3 })).toEqual({
      kind: "move",
      index: 0,
    })
    expect(inlineKeyStep({ key: "ArrowUp", active: 0, count: 3 })).toEqual({
      kind: "move",
      index: 2,
    })
  })

  it("closes on Escape even with no matches", () => {
    expect(inlineKeyStep({ key: "Escape", active: -1, count: 0 })).toEqual({
      kind: "close",
    })
  })

  it("ignores other keys", () => {
    expect(inlineKeyStep({ key: "a", active: 0, count: 3 })).toEqual({
      kind: "pass",
    })
  })
})
