import { describe, expect, test } from "vitest"

import {
  auditKinds,
  describeAuditEvent,
  rangeSince,
  type ActivityEvent,
} from "./audit-events"

function event(type: ActivityEvent["type"], data: object): ActivityEvent {
  return { type, data } as ActivityEvent
}

describe("rangeSince", () => {
  const now = Date.parse("2026-11-19T12:00:00Z")

  test("counts back from now", () => {
    expect(rangeSince("hour", now)).toBe("2026-11-19T11:00:00.000Z")
    expect(rangeSince("day", now)).toBe("2026-11-18T12:00:00.000Z")
    expect(rangeSince("week", now)).toBe("2026-11-12T12:00:00.000Z")
    expect(rangeSince("month", now)).toBe("2026-10-20T12:00:00.000Z")
  })

  test("sends no bound for all time", () => {
    expect(rangeSince("all", now)).toBeUndefined()
  })
})

describe("auditKinds", () => {
  test("lists each kind once", () => {
    const values = auditKinds.map((kind) => kind.value)
    expect(new Set(values).size).toBe(values.length)
  })

  test("describes every kind in words, not the raw type", () => {
    for (const kind of auditKinds) {
      expect(describeAuditEvent(event(kind.value, {}))).not.toBe(kind.value)
    }
  })
})

describe("describeAuditEvent", () => {
  test("says what a role change was", () => {
    expect(
      describeAuditEvent(
        event("user.role_changed", {
          name: "Asha",
          from_role: "viewer",
          to_role: "editor",
          manual: true,
        })
      )
    ).toBe("Changed the role of Asha from viewer to editor")
  })

  test("says when a role override was cleared", () => {
    expect(
      describeAuditEvent(
        event("user.role_changed", {
          name: "Asha",
          from_role: "editor",
          to_role: "viewer",
          manual: false,
        })
      )
    ).toBe(
      "Cleared the role override of Asha; the role is now viewer (was editor)"
    )
  })

  test("lists only what changed on a service account", () => {
    expect(
      describeAuditEvent(
        event("service_account.updated", {
          name: "ci",
          role: { from: "viewer", to: "editor" },
        })
      )
    ).toBe('Changed the service account "ci": role viewer to editor')
  })

  test("names a key without showing a value", () => {
    expect(
      describeAuditEvent(
        event("service_account.key_created", {
          name: "ci",
          key_name: "deploy",
          key_id: "k1",
        })
      )
    ).toBe('Created the key "deploy" for the service account "ci"')
  })

  test("notes a replaced Grafana token without the token", () => {
    expect(
      describeAuditEvent(
        event("grafana_connection.updated", {
          name: "prod",
          token_changed: true,
        })
      )
    ).toBe('Changed the Grafana connection "prod": token replaced')
  })

  test("calls an observation link an RCA", () => {
    expect(
      describeAuditEvent(
        event("public_link.created", {
          object: { kind: "observation", id: "o1" },
          title: "Checkout slowdown",
        })
      )
    ).toBe('Created a public link to the RCA "Checkout slowdown"')
  })

  test("falls back to the type for one it does not know", () => {
    expect(describeAuditEvent(event("run.failed", {}))).toBe("run.failed")
  })
})
