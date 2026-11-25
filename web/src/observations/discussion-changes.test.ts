import { describe, expect, it } from "vitest"

import {
  blockSubject,
  discussionToggleLabel,
  observationChanges,
} from "./discussion-changes"
import type { ActivityEvent } from "@/live/activity-match"

import type { Block, Observation } from "./queries"

const ada = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

const agent = {
  ...ada,
  id: "u2",
  name: "Grace",
  run_id: "r1",
  label: "Observe Agent for Grace",
}

function block(id: string, overrides: Partial<Block> = {}): Block {
  return {
    id,
    observation_id: "o1",
    kind: "text",
    state: "live",
    position: 0,
    width: "full",
    height: null,
    version: 1,
    body: { markdown: id },
    snapshot_id: null,
    run_id: null,
    created_by: ada,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: [],
    ...overrides,
  }
}

function observation(blocks?: Block[]): Observation {
  return {
    id: "o1",
    title: "Checkout errors",
    state: "open",
    created_by: ada,
    created_at: "2026-11-12T09:00:00Z",
    blocks,
  } as Observation
}

const noContext = { folderTitle: "My Dashboards", datasources: [], events: [] }

function event(
  seq: number,
  type: ActivityEvent["type"],
  data: Record<string, unknown>,
  actor: ActivityEvent["actor"] = ada
): ActivityEvent {
  return {
    seq,
    type,
    target: { kind: "observation", id: "o1" },
    dashboard_id: null,
    folder_id: null,
    actor,
    data,
    created_at: `2026-11-12T12:0${seq}:00Z`,
  } as unknown as ActivityEvent
}

describe("observationChanges", () => {
  it("starts with who started the observation, without a subject", () => {
    expect(observationChanges(observation([]), noContext)).toEqual([
      {
        key: "started",
        at: "2026-11-12T09:00:00Z",
        actor: ada,
        verb: "started this observation",
        subject: null,
        detail: "Checkout errors · In My Dashboards",
      },
    ])
  })

  it("reads a response with no blocks as only the start", () => {
    expect(observationChanges(observation(), noContext)).toHaveLength(1)
  })

  it("adds one entry per block, suggested when the block is requested", () => {
    const changes = observationChanges(
      observation([
        block("b1", { created_at: "2026-11-12T10:00:00Z" }),
        block("b2", {
          state: "requested",
          created_by: agent,
          created_at: "2026-11-12T11:00:00Z",
        }),
      ]),
      noContext
    )
    expect(changes.slice(1)).toEqual([
      {
        key: "b1",
        at: "2026-11-12T10:00:00Z",
        actor: ada,
        verb: "added a block",
        subject: { id: "b1", title: "b1" },
        detail: null,
      },
      {
        key: "b2",
        at: "2026-11-12T11:00:00Z",
        actor: agent,
        verb: "suggested a block",
        subject: { id: "b2", title: "b2" },
        detail: null,
      },
    ])
  })
})

describe("observationChanges block lines", () => {
  const timeseries = (body: Record<string, unknown>) =>
    block("p1", {
      kind: "panel",
      body: { panel: { type: "timeseries", title: "Latency", ...body } },
    })
  const datasources = [{ uid: "prom", name: "Prometheus" }]

  function lineOf(target: Block, known = datasources) {
    const changes = observationChanges(observation([target]), {
      ...noContext,
      datasources: known,
    })
    return changes[1].detail
  }

  it("names the datasource of a panel block and says it is live", () => {
    expect(lineOf(timeseries({ datasource: "prom", queries: [] }))).toBe(
      "Prometheus, live"
    )
  })

  it("reads the datasource from a query when the panel names none", () => {
    expect(
      lineOf(timeseries({ queries: [{ refId: "A", datasource: "prom" }] }))
    ).toBe("Prometheus, live")
  })

  it("falls back to the uid for a datasource it cannot list", () => {
    expect(lineOf(timeseries({ datasource: "gone", queries: [] }))).toBe(
      "gone, live"
    )
  })

  it("says only live for a panel with no datasource", () => {
    expect(lineOf(timeseries({ queries: [] }))).toBe("live")
  })

  it("has no line for a text block or an unreadable panel block", () => {
    expect(lineOf(block("t1"))).toBeNull()
    expect(
      lineOf(block("p2", { kind: "panel", body: { panel: "broken" } }))
    ).toBeNull()
  })
})

describe("observationChanges from activity", () => {
  function fromEvents(events: ActivityEvent[]) {
    return observationChanges(observation([]), { ...noContext, events }).slice(
      1
    )
  }

  it("draws a removed block unnamed, by who removed it", () => {
    const changes = fromEvents([
      event(
        3,
        "observation.block_removed",
        { title: "Checkout errors", block_id: "b1", block_kind: "text" },
        agent
      ),
    ])
    expect(changes).toEqual([
      {
        key: "event:3",
        at: "2026-11-12T12:03:00Z",
        actor: agent,
        verb: "removed a block",
        subject: null,
        detail: null,
      },
    ])
  })

  it("draws a rename only when the title differs from the one before", () => {
    const changes = fromEvents([
      event(4, "observation.updated", { title: "Checkout 5xx" }),
      event(1, "observation.created", { title: "Checkout errors" }),
      event(2, "observation.updated", { title: "Checkout errors" }),
      event(3, "observation.updated", { title: "Checkout 5xx after deploy" }),
    ])
    expect(changes.map((change) => change.verb)).toEqual([
      "renamed this observation to Checkout 5xx after deploy",
      "renamed this observation to Checkout 5xx",
    ])
  })

  it("names the title a rename started from and starts the page under it", () => {
    const changes = observationChanges(
      { ...observation([]), title: "Checkout 5xx" },
      {
        ...noContext,
        events: [
          event(2, "observation.updated", {
            title: "Checkout 5xx",
            from_title: "Checkout errors",
          }),
        ],
      }
    )
    expect(changes[0].detail).toBe("Checkout errors · In My Dashboards")
    expect(changes[1].verb).toBe(
      "renamed this observation from Checkout errors to Checkout 5xx"
    )
  })

  it("starts the page under the title it was created with", () => {
    const changes = observationChanges(
      { ...observation([]), title: "Renamed" },
      {
        ...noContext,
        events: [event(1, "observation.created", { title: "First title" })],
      }
    )
    expect(changes[0].detail).toBe("First title · In My Dashboards")
  })

  it("draws no rename for the first update when the start is not known", () => {
    expect(
      fromEvents([event(2, "observation.updated", { title: "New name" })])
    ).toEqual([])
  })
})

describe("blockSubject", () => {
  it("names a titled panel, an untitled panel and the other kinds", () => {
    const titled = block("b1", {
      kind: "panel",
      body: { panel: { title: "Latency" } },
    })
    const untitled = block("b2", { kind: "panel", body: { panel: {} } })
    expect(blockSubject(titled)).toBe("Panel: Latency")
    expect(blockSubject(untitled)).toBe("Panel")
    expect(blockSubject(block("b3"))).toBe("b3")
    expect(blockSubject(block("b5", { body: { markdown: "" } }))).toBe("Text")
    const snapshot = block("b4", {
      kind: "snapshot",
      body: { snapshot_id: "s1" },
    })
    expect(blockSubject(snapshot)).toBe("Snapshot")
    expect(blockSubject(snapshot, new Map([["s1", "Checkout latency"]]))).toBe(
      "Snapshot: Checkout latency"
    )
    expect(blockSubject(block("b4", { kind: "snapshot" }))).toBe("Snapshot")
    expect(blockSubject(block("b5", { kind: "run" }))).toBe("Run")
  })
})

describe("discussionToggleLabel", () => {
  it.each([
    [0, "Discussion"],
    [1, "Discussion, 1 message"],
    [7, "Discussion, 7 messages"],
  ])("labels %i messages", (count, label) => {
    expect(discussionToggleLabel(count)).toBe(label)
  })
})

describe("observationChanges state and block events", () => {
  it("does not state the live state on the start line of a resolved observation", () => {
    const resolved = { ...observation([]), state: "resolved" } as Observation
    expect(observationChanges(resolved, noContext)[0].detail).toBe(
      "Checkout errors · In My Dashboards"
    )
  })

  it("lists resolving, reopening, publishing, withdrawing and moving in order", () => {
    const changes = observationChanges(observation([]), {
      ...noContext,
      events: [
        event(5, "observation.rca_withdrawn", { title: "T", revision: 1 }),
        event(4, "observation.published", { title: "T", revision: 2 }),
        event(3, "observation.reopened", { title: "T" }),
        event(2, "observation.resolved", { title: "T" }),
        event(1, "observation.moved", { title: "T" }),
      ],
    })
    expect(changes.slice(1).map((change) => change.verb)).toEqual([
      "moved this observation to another folder",
      "resolved this observation",
      "reopened this observation",
      "published revision 2 as an RCA",
      "withdrew revision 1 of the RCA",
    ])
  })

  it("names the block that was edited and leaves the subject off once it is gone", () => {
    const changes = observationChanges(observation([block("b1")]), {
      ...noContext,
      events: [
        event(1, "observation.block_updated", { block_id: "b1" }),
        event(2, "observation.block_updated", { block_id: "gone" }),
      ],
    })
    const edits = changes.filter((change) => change.verb === "changed a block")
    expect(edits.map((change) => change.subject)).toEqual([
      { id: "b1", title: "b1" },
      null,
    ])
  })
})
