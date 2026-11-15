import { describe, expect, it } from "vitest"

import {
  blockSubject,
  discussionToggleLabel,
  observationChanges,
} from "./discussion-changes"
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
    created_by: ada,
    created_at: "2026-11-12T09:00:00Z",
    blocks,
  } as Observation
}

describe("observationChanges", () => {
  it("starts with who started the observation, without a subject", () => {
    expect(observationChanges(observation([]))).toEqual([
      {
        key: "started",
        at: "2026-11-12T09:00:00Z",
        actor: ada,
        verb: "started this observation",
        subject: null,
      },
    ])
  })

  it("reads a response with no blocks as only the start", () => {
    expect(observationChanges(observation())).toHaveLength(1)
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
      ])
    )
    expect(changes.slice(1)).toEqual([
      {
        key: "b1",
        at: "2026-11-12T10:00:00Z",
        actor: ada,
        verb: "added a block",
        subject: { id: "b1", title: "Text" },
      },
      {
        key: "b2",
        at: "2026-11-12T11:00:00Z",
        actor: agent,
        verb: "suggested a block",
        subject: { id: "b2", title: "Text" },
      },
    ])
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
    expect(blockSubject(block("b3"))).toBe("Text")
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
