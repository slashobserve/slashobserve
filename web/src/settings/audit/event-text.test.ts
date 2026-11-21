import { describe, expect, test } from "vitest"

import {
  actorText,
  eventData,
  eventFields,
  originText,
  targetText,
} from "./event-text"
import type { AuditEvent } from "./stream"

function event(fields: Partial<AuditEvent>): AuditEvent {
  return {
    id: "e1",
    seq: 7,
    occurred_at: "2026-11-21T06:00:00Z",
    action: "dashboard.update",
    outcome: "success",
    actor_kind: "user",
    actor_name: null,
    origin: "web",
    origin_client: null,
    target_name: null,
    target_id: null,
    reason: "",
    data: {},
    ...fields,
  } as AuditEvent
}

describe("event text", () => {
  test("names the actor, or says who it was without a name", () => {
    expect(actorText(event({ actor_name: "Zoë" }))).toBe("Zoë")
    expect(actorText(event({ actor_kind: "system" }))).toBe("System")
    expect(actorText(event({ actor_kind: "anonymous" }))).toBe("Anonymous")
  })

  test("prefers the target's name to its id", () => {
    expect(
      targetText(event({ target_name: "Checkout", target_id: "d1" }))
    ).toBe("Checkout")
    expect(targetText(event({ target_id: "d1" }))).toBe("d1")
    expect(targetText(event({}))).toBe("")
  })

  test("adds the connected client to the origin", () => {
    expect(originText(event({ origin: "mcp", origin_client: "Claude" }))).toBe(
      "mcp (Claude)"
    )
    expect(originText(event({ origin: "cli" }))).toBe("cli")
  })
})

describe("eventFields", () => {
  test("lists every field with a value, leaving data to its own block", () => {
    expect(
      eventFields(event({ actor_name: "Zoë", data: { a: 1 } })).map((field) => [
        field.name,
        field.label,
        field.value,
      ])
    ).toEqual([
      ["id", "Id", "e1"],
      ["seq", "Seq", "7"],
      ["occurred_at", "Occurred at", "2026-11-21T06:00:00Z"],
      ["action", "Action", "dashboard.update"],
      ["outcome", "Outcome", "success"],
      ["actor_kind", "Actor kind", "user"],
      ["actor_name", "Actor name", "Zoë"],
      ["origin", "Origin", "web"],
    ])
  })
})

describe("eventData", () => {
  test("prints data as JSON and skips an empty object", () => {
    expect(eventData(event({ data: { rows: 3 } }))).toBe('{\n  "rows": 3\n}')
    expect(eventData(event({ data: {} }))).toBe(null)
  })
})
