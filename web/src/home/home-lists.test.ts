import { describe, expect, test } from "vitest"

import type { InboxItem } from "@/inbox/inbox-item"

import {
  agentMark,
  createdBy,
  elapsedText,
  importMeta,
  messageCountText,
  needsYou,
  pickUp,
  runningText,
  workingPlaces,
  workingTarget,
  workingTitle,
  type WorkingRun,
} from "./home-lists"

function inboxItem(seq: number, reason: InboxItem["reason"]): InboxItem {
  return { seq, reason } as InboxItem
}

function viewed(id: string, viewedAt: string) {
  return { id, title: id, viewed_at: viewedAt }
}

describe("needsYou", () => {
  test("drops items with no reason and cuts to the limit", () => {
    const items = [
      inboxItem(1, null),
      inboxItem(2, "mention"),
      inboxItem(3, "reply"),
      inboxItem(4, null),
      inboxItem(5, "proposal"),
      inboxItem(6, "run_failed"),
    ]
    expect(needsYou(items, 3).map((item) => item.seq)).toEqual([2, 3, 5])
  })
})

describe("createdBy", () => {
  test("keeps only the person's own items and cuts to the limit", () => {
    const items = [
      { id: "a", created_by: { id: "me" } },
      { id: "b", created_by: { id: "other" } },
      { id: "c", created_by: { id: "me" } },
      { id: "d", created_by: { id: "me" } },
    ]
    expect(createdBy(items, "me", 2).map((item) => item.id)).toEqual(["a", "c"])
  })
})

describe("pickUp", () => {
  test("interleaves dashboards and observations by when they were viewed", () => {
    const dashboards = [
      viewed("d1", "2026-11-06T12:00:00Z"),
      viewed("d2", "2026-11-06T08:00:00Z"),
      viewed("d3", "2026-11-05T08:00:00Z"),
    ]
    const observations = [
      viewed("o1", "2026-11-06T10:00:00Z"),
      viewed("o2", "2026-11-06T06:00:00Z"),
      viewed("o3", "2026-11-04T06:00:00Z"),
    ]
    expect(
      pickUp(dashboards, observations, 5).map((e) => [e.kind, e.id])
    ).toEqual([
      ["dashboard", "d1"],
      ["observation", "o1"],
      ["dashboard", "d2"],
      ["observation", "o2"],
      ["dashboard", "d3"],
    ])
  })
})

function working(overrides: Partial<WorkingRun> = {}): WorkingRun {
  return {
    id: "run1",
    intent: "apply",
    source: "prompt",
    state: "running",
    created_at: "2026-11-15T10:00:00Z",
    started_at: "2026-11-15T10:00:00Z",
    mine: true,
    title: "Add a latency row",
    agent: { kind: "builtin", name: "Observe Agent" },
    requested_by: { kind: "user", name: "admin" },
    dashboard: null,
    observation: null,
    ...overrides,
  }
}

const shop = { id: "d1", title: "Demo shop overview" }
const incident = { id: "o1", title: "Checkout latency" }

describe("workingTitle", () => {
  test("uses the request's own title for a run you started", () => {
    expect(workingTitle(working({ dashboard: shop }))).toBe("Add a latency row")
  })

  test("names the dashboard, then the observation, of someone else's run", () => {
    const other = { mine: false, title: null }
    expect(workingTitle(working({ ...other, dashboard: shop }))).toBe(
      "Demo shop overview"
    )
    expect(workingTitle(working({ ...other, observation: incident }))).toBe(
      "Checkout latency"
    )
  })

  test("falls back when the run's thing is hidden", () => {
    expect(workingTitle(working({ mine: false, title: null }))).toBe(
      "something of yours"
    )
  })
})

describe("workingPlaces", () => {
  test("lists the observation and the dashboard of a run you started", () => {
    expect(
      workingPlaces(working({ dashboard: shop, observation: incident }))
    ).toEqual(["in Checkout latency", "on Demo shop overview"])
  })

  test("leaves out the thing the title already names", () => {
    const other = { mine: false, title: null }
    expect(workingPlaces(working({ ...other, dashboard: shop }))).toEqual([])
    expect(
      workingPlaces(
        working({ ...other, dashboard: shop, observation: incident })
      )
    ).toEqual(["in Checkout latency"])
  })

  test("is empty without a dashboard or observation", () => {
    expect(workingPlaces(working())).toEqual([])
  })
})

describe("workingTarget", () => {
  test("opens the chat for a run you started", () => {
    expect(workingTarget(working({ dashboard: shop }))).toEqual({
      kind: "chat",
      id: "run1",
    })
  })

  test("opens the dashboard, else the observation, of someone else's run", () => {
    const other = { mine: false, title: null }
    expect(workingTarget(working({ ...other, dashboard: shop }))).toEqual({
      kind: "dashboard",
      id: "d1",
      title: "Demo shop overview",
    })
    expect(workingTarget(working({ ...other, observation: incident }))).toEqual(
      {
        kind: "observation",
        id: "o1",
      }
    )
    expect(workingTarget(working(other))).toBeNull()
  })
})

describe("agentMark", () => {
  test("gives the built-in agent its own mark and others an outside-agent mark", () => {
    expect(agentMark({ kind: "builtin", name: "Observe Agent" }).kind).toBe(
      "observe-agent"
    )
    expect(agentMark({ kind: "grant", name: "Claude Code" })).toEqual({
      kind: "outside-agent",
      text: "CC",
      tint: null,
    })
  })
})

describe("elapsedText", () => {
  test.each([
    [-5000, "0 s"],
    [12_000, "12 s"],
    [60_000, "1 min"],
    [125_000, "2 min"],
    [3_600_000, "1 h"],
    [3_960_000, "1 h 6 min"],
  ])("%d ms reads %s", (ms, text) => {
    expect(elapsedText(ms)).toBe(text)
  })
})

describe("runningText", () => {
  const now = Date.parse("2026-11-15T10:02:10Z")

  test("counts from when the run started", () => {
    expect(runningText(working(), now)).toBe("Running, 2 min")
  })

  test("says Queued before it starts", () => {
    expect(
      runningText(working({ state: "queued", started_at: null }), now)
    ).toBe("Queued")
  })
})

describe("messageCountText", () => {
  test.each([
    [0, null],
    [1, "1 message"],
    [4, "4 messages"],
  ])("%d reads %s", (count, text) => {
    expect(messageCountText(count)).toBe(text)
  })
})

describe("importMeta", () => {
  const counts = {
    total: 18,
    imported: 16,
    partial: 0,
    placeholder: 2,
    unverified: 0,
  }

  test("adds the placeholders to the panel count and drops the duplicate status", () => {
    expect(importMeta(counts)).toEqual({
      status: null,
      counts: "18 panels, 2 placeholders",
    })
  })

  test("keeps the status of an import with unverified panels", () => {
    expect(importMeta({ ...counts, unverified: 1 })).toEqual({
      status: { tone: "warning", text: "1 unverified" },
      counts: "18 panels, 2 placeholders",
    })
  })

  test("is plain for a clean import", () => {
    expect(
      importMeta({
        total: 1,
        imported: 1,
        partial: 0,
        placeholder: 0,
        unverified: 0,
      })
    ).toEqual({ status: { tone: "ok", text: "Imported" }, counts: "1 panel" })
  })
})
