import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "../transforms"
import { activeFor, alertModel, missingAlertsReason } from "./alert-model"

const now = Date.parse("2026-10-28T12:00:00Z")

function instant(
  name: string,
  labels: Record<string, string>,
  value = 1
): PreparedFrame {
  return {
    ref: name,
    name,
    labels,
    fields: [
      { name: "Time", type: "time", values: [now] },
      { name: "Value", type: "number", values: [value] },
    ],
  } as PreparedFrame
}

function alert(
  alertname: string,
  alertstate: string,
  extra: Record<string, string> = {}
): PreparedFrame {
  return instant("ALERTS", { alertname, alertstate, ...extra })
}

function activeSince(
  alertname: string,
  secondsAgo: number,
  extra: Record<string, string> = {}
): PreparedFrame {
  return instant(
    "ALERTS_FOR_STATE",
    { alertname, ...extra },
    now / 1000 - secondsAgo
  )
}

function names(frames: PreparedFrame[], display: Record<string, unknown>) {
  return alertModel(frames, display, now).alerts.map((item) => item.name)
}

describe("alertModel", () => {
  it("lists firing and pending alerts with their labels", () => {
    const model = alertModel(
      [
        alert("HighErrorRate", "firing", {
          severity: "critical",
          service: "checkout",
        }),
        alert("SlowRequests", "pending", { severity: "warning" }),
      ],
      {},
      now
    )
    expect(model.reason).toBeNull()
    expect(model.alerts).toMatchObject([
      {
        name: "HighErrorRate",
        state: "firing",
        severity: "critical",
        labels: [["service", "checkout"]],
      },
      { name: "SlowRequests", state: "pending", severity: "warning" },
    ])
  })

  it("skips series in any other alert state", () => {
    expect(names([alert("A", "firing"), alert("B", "inactive")], {})).toEqual([
      "A",
    ])
  })

  it("joins the active-since time from ALERTS_FOR_STATE by labels", () => {
    const model = alertModel(
      [
        alert("A", "firing", { service: "checkout" }),
        alert("A", "firing", { service: "payments" }),
        activeSince("A", 720, { service: "checkout" }),
      ],
      {},
      now
    )
    expect(
      model.alerts.map((item) => [item.labels[0][1], item.activeFor])
    ).toEqual([
      ["checkout", "for 12m"],
      ["payments", null],
    ])
    expect(model.alerts[0].since).toBe(now - 720_000)
  })

  it("sorts A to Z and Z to A by name", () => {
    const frames = [
      alert("B", "firing"),
      alert("C", "firing"),
      alert("A", "firing"),
    ]
    expect(names(frames, { sort: "alphaAsc" })).toEqual(["A", "B", "C"])
    expect(names(frames, { sort: "alphaDesc" })).toEqual(["C", "B", "A"])
  })

  it("sorts by importance: firing first, then severity, then name", () => {
    const frames = [
      alert("A", "pending", { severity: "critical" }),
      alert("B", "firing", { severity: "info" }),
      alert("C", "firing", { severity: "critical" }),
      alert("D", "firing"),
      alert("E", "firing", { severity: "warning" }),
      alert("F", "firing", { severity: "error" }),
    ]
    expect(names(frames, { sort: "importance" })).toEqual([
      "C",
      "F",
      "E",
      "B",
      "D",
      "A",
    ])
  })

  it("sorts by time with alerts that have no since time last", () => {
    const frames = [
      alert("A", "firing"),
      alert("B", "firing"),
      alert("C", "firing"),
      alert("D", "firing"),
      activeSince("B", 600),
      activeSince("C", 60),
      activeSince("D", 3600),
    ]
    expect(names(frames, { sort: "timeAsc" })).toEqual(["D", "B", "C", "A"])
    expect(names(frames, { sort: "timeDesc" })).toEqual(["C", "B", "D", "A"])
  })

  it("limits the list and reports the total", () => {
    const frames = [
      alert("A", "firing"),
      alert("B", "firing"),
      alert("C", "firing"),
    ]
    const model = alertModel(frames, { limit: 2 }, now)
    expect(model.alerts).toHaveLength(2)
    expect(model.total).toBe(3)
  })

  it("counts every alert by state, not only the limited ones", () => {
    const model = alertModel(
      [alert("A", "firing"), alert("B", "firing"), alert("C", "pending")],
      { limit: 1 },
      now
    )
    expect(model.counts).toEqual({ firing: 2, pending: 1 })
  })

  it("is an empty list, without a reason, when there are no frames", () => {
    const model = alertModel([], {}, now)
    expect(model).toEqual({
      reason: null,
      alerts: [],
      total: 0,
      counts: { firing: 0, pending: 0 },
    })
  })

  it("explains frames that are not on ALERTS", () => {
    const model = alertModel([instant("up", { job: "shop" })], {}, now)
    expect(model.reason).toBe(missingAlertsReason)
    expect(model.alerts).toEqual([])
  })
})

describe("activeFor", () => {
  it("uses the largest whole unit", () => {
    expect(activeFor(now - 5_000, now)).toBe("for 5s")
    expect(activeFor(now - 12 * 60_000, now)).toBe("for 12m")
    expect(activeFor(now - 3 * 3_600_000, now)).toBe("for 3h")
    expect(activeFor(now - 2 * 86_400_000, now)).toBe("for 2d")
  })

  it("never goes negative when the clock is behind the alert", () => {
    expect(activeFor(now + 5_000, now)).toBe("for 0s")
  })
})
