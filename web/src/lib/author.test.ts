import { describe, expect, it } from "vitest"

import { markOf, personTint, type Actor } from "./author"

function actor(overrides: Partial<Actor> = {}): Actor {
  return {
    kind: "user",
    id: "6f1c1b0e-7a53-4c55-9d52-0a3d8a8d5b11",
    name: "Priya Raman",
    origin: "web",
    client: null,
    run_id: null,
    label: "Priya Raman",
    ...overrides,
  }
}

describe("markOf", () => {
  it("gives a person a tinted mark with their initials", () => {
    const mark = markOf(actor())
    expect(mark.kind).toBe("person")
    expect(mark.text).toBe("PR")
    expect(mark.tint).toBe(personTint(actor().id))
  })

  it("gives a run the slash of the built-in agent", () => {
    expect(markOf(actor({ run_id: "run-1" }))).toEqual({
      kind: "observe-agent",
      text: "/",
      tint: null,
    })
  })

  it("lets a run win over an MCP origin", () => {
    const mark = markOf(
      actor({ run_id: "run-1", origin: "mcp", client: "Codex" })
    )
    expect(mark.kind).toBe("observe-agent")
  })

  it("draws an MCP client outlined with the client's initials", () => {
    expect(markOf(actor({ origin: "mcp", client: "Claude Code" }))).toEqual({
      kind: "outside-agent",
      text: "CC",
      tint: null,
    })
  })

  it("falls back to MCP when the client sent no name", () => {
    expect(markOf(actor({ origin: "mcp", client: null })).text).toBe("MC")
  })

  it("shows a question mark when the client name is empty", () => {
    expect(markOf(actor({ origin: "mcp", client: "" })).text).toBe("?")
  })

  it("draws a service account outlined with its initials", () => {
    expect(
      markOf(
        actor({ kind: "service_account", name: "ci-deploys", origin: "api" })
      )
    ).toEqual({ kind: "outside-agent", text: "CD", tint: null })
  })
})

describe("personTint", () => {
  it("is stable for one id", () => {
    expect(personTint("abc")).toBe(personTint("abc"))
  })

  it("follows the code unit sum modulo six, plus one", () => {
    expect(personTint("a")).toBe(2)
    expect(personTint("")).toBe(1)
  })

  it("stays within one to six for different ids", () => {
    const ids = [
      "0b7a6d1e-52c8-4a3f-9e0b-1d2c3b4a5f60",
      "1c8b7e2f-63d9-4b40-8f1c-2e3d4c5b6a71",
      "2d9c8f30-74ea-4c51-9a2d-3f4e5d6c7b82",
      "3eadf041-85fb-4d62-8b3e-405f6e7d8c93",
      "4fbe0152-96ac-4e73-9c4f-516a7f8e9da4",
      "50cf1263-a7bd-4f84-8d50-627b8a9fa0b5",
      "61d02374-b8ce-4095-9e61-738c9bafb1c6",
      "72e13485-c9df-41a6-8f72-849dacb0c2d7",
      "83f24596-daf0-42b7-9a83-95aebdc1d3e8",
      "94035607-ebf1-43c8-8b94-a6bfced2e4f9",
    ]
    for (const id of ids) {
      const tint = personTint(id)
      expect(tint).toBeGreaterThanOrEqual(1)
      expect(tint).toBeLessThanOrEqual(6)
    }
  })
})
