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

  test("says when the sign-in policy changed a role", () => {
    expect(
      describeAuditEvent(
        event("user.role_changed", {
          name: "Asha",
          from_role: "editor",
          to_role: "admin",
          manual: false,
          source: "group",
        })
      )
    ).toBe(
      "The sign-in policy changed the role of Asha from editor to admin (group)"
    )
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

  test("says how an invite's role changed", () => {
    expect(
      describeAuditEvent(
        event("user.invite_updated", {
          email: "asha@example.com",
          role: { from: "editor", to: "viewer" },
        })
      )
    ).toBe("Changed the invite of asha@example.com: role editor to viewer")
  })

  test("counts what a deactivation ended", () => {
    expect(
      describeAuditEvent(
        event("user.deactivated", { name: "Asha", sessions: 2, grants: 1 })
      )
    ).toBe("Deactivated Asha (2 sessions, 1 connected agents)")
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

  test("summarizes a webhook change without the address path", () => {
    expect(
      describeAuditEvent(
        event("webhook.updated", {
          name: "SIEM",
          host: "hooks.example.com",
          url_changed: true,
          changes: {
            enabled: { from: false, to: true },
            event_types: { from: ["a.b"], to: ["a.b", "c.d"] },
          },
        })
      )
    ).toBe(
      'Changed the webhook "SIEM": enabled false to true, event types 1 to 2, address path'
    )
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

  test("describes a datasource change without credentials", () => {
    expect(
      describeAuditEvent(
        event("datasource.updated", {
          name: "Metrics",
          host: "prom2.example.com",
          address_changed: true,
          credentials_changed: true,
          changes: {
            host: { from: "prom.example.com", to: "prom2.example.com" },
          },
        })
      )
    ).toBe(
      'Changed the datasource "Metrics": host prom.example.com to prom2.example.com, address changed, credentials replaced'
    )
    expect(
      describeAuditEvent(
        event("datasource.updated", {
          name: "Metrics",
          host: "prom.example.com",
          changes: { audience: { from: "everyone", to: "admins" } },
        })
      )
    ).toBe('Changed the datasource "Metrics": audience everyone to admins')
    expect(
      describeAuditEvent(
        event("datasource.created", {
          name: "Metrics",
          host: "prom.example.com",
        })
      )
    ).toBe('Added the datasource "Metrics" at prom.example.com')
    expect(
      describeAuditEvent(event("datasource.deleted", { name: "Metrics" }))
    ).toBe('Deleted the datasource "Metrics"')
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

  test("names the object and the requester of comment access", () => {
    expect(
      describeAuditEvent(
        event("access_request.opened", {
          object: { kind: "observation", id: "o1" },
          title: "",
        })
      )
    ).toBe("Asked for comment access to an observation")
    expect(
      describeAuditEvent(
        event("access_request.granted", {
          object: { kind: "dashboard", id: "d1" },
          title: "Checkout",
          requested_by: { kind: "user", id: "u1", name: "Ben" },
        })
      )
    ).toBe(
      'Granted comment access to the dashboard "Checkout" asked for by Ben'
    )
  })

  test("falls back to the type for one it does not know", () => {
    expect(describeAuditEvent(event("run.failed", {}))).toBe("run.failed")
  })
})
