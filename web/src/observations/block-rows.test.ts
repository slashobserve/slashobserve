import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"
import type { DataPanel } from "@/spec"

import {
  addProblemText,
  blockRows,
  blockTitle,
  newPanelBody,
  nextBlockId,
} from "./block-rows"
import type { Block } from "./queries"

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function block(
  id: string,
  position: number,
  width: Block["width"] = "full",
  overrides: Partial<Block> = {}
): Block {
  return {
    id,
    observation_id: "o1",
    kind: "text",
    state: "live",
    position,
    width,
    version: 1,
    body: { markdown: id },
    snapshot_id: null,
    run_id: null,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: [],
    ...overrides,
  }
}

function shape(blocks: Block[]): string[][] {
  return blockRows(blocks).map((row) => row.blocks.map((b) => b.id))
}

describe("blockRows", () => {
  it("keeps a full block on its own row and pairs two half blocks", () => {
    const rows = shape([
      block("a", 0, "full"),
      block("b", 1, "half"),
      block("c", 2, "half"),
      block("d", 3, "full"),
    ])
    expect(rows).toEqual([["a"], ["b", "c"], ["d"]])
  })

  it("leaves a half block before a full block on its own row", () => {
    expect(shape([block("a", 0, "half"), block("b", 1, "full")])).toEqual([
      ["a"],
      ["b"],
    ])
  })

  it("pairs half blocks from the start and leaves the odd one last", () => {
    expect(
      shape([
        block("a", 0, "half"),
        block("b", 1, "half"),
        block("c", 2, "half"),
      ])
    ).toEqual([["a", "b"], ["c"]])
  })

  it("gives no rows for no blocks", () => {
    expect(blockRows([])).toEqual([])
  })

  it("orders by position and keys a row by its first block", () => {
    const rows = blockRows([block("b", 1, "half"), block("a", 0, "half")])
    expect(rows).toHaveLength(1)
    expect(rows[0].key).toBe("a")
    expect(rows[0].blocks.map((b) => b.id)).toEqual(["a", "b"])
  })
})

describe("blockTitle", () => {
  it("names a panel block by its panel title", () => {
    const titled = block("p", 0, "full", {
      kind: "panel",
      body: { panel: { type: "stat", title: "Error rate" } },
    })
    expect(blockTitle(titled)).toBe("Error rate")
  })

  it("falls back to the kind when a panel has no title", () => {
    const untitled = block("p", 0, "full", {
      kind: "panel",
      body: { panel: { type: "stat" } },
    })
    expect(blockTitle(untitled)).toBe("Panel")
    expect(blockTitle(block("t", 0))).toBe("Text")
    expect(blockTitle(block("s", 0, "full", { kind: "snapshot" }))).toBe(
      "Snapshot"
    )
    expect(blockTitle(block("r", 0, "full", { kind: "run" }))).toBe("Run")
  })
})

describe("nextBlockId", () => {
  const blocks = [block("b", 1), block("a", 0), block("c", 2)]

  it("names the block that follows in page order", () => {
    expect(nextBlockId(blocks, "a")).toBe("b")
    expect(nextBlockId(blocks, "b")).toBe("c")
  })

  it("names nothing after the last block or an unknown block", () => {
    expect(nextBlockId(blocks, "c")).toBeNull()
    expect(nextBlockId(blocks, "zzz")).toBeNull()
  })
})

describe("newPanelBody", () => {
  const panel = {
    id: "latency",
    type: "stat",
    title: "Latency",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    queries: [{ query: "up" }],
  } as unknown as DataPanel

  it("strips the dashboard placement and carries the observation range", () => {
    const body = newPanelBody(panel, {
      from: "2026-11-12T09:00:00Z",
      to: "2026-11-12T10:00:00Z",
    })
    expect(body).toEqual({
      panel: { type: "stat", title: "Latency", queries: [{ query: "up" }] },
      from: "2026-11-12T09:00:00Z",
      to: "2026-11-12T10:00:00Z",
    })
  })

  it("sends no range when the observation has none", () => {
    const body = newPanelBody(panel, { from: null, to: null })
    expect(body).not.toHaveProperty("from")
    expect(body).not.toHaveProperty("to")
  })
})

describe("addProblemText", () => {
  it.each([
    [
      new ApiError(429, "rate_limited", "Slow down", [], undefined, 7000),
      "Too many requests right now. Try again in 7 seconds.",
    ],
    [
      new ApiError(429, "rate_limited", "Slow down", [], undefined, 200),
      "Too many requests right now. Try again in 1 second.",
    ],
    [
      new ApiError(429, "rate_limited", "Slow down"),
      "Too many requests right now. Try again shortly.",
    ],
    [
      new ApiError(502, "datasource_unavailable", "Down"),
      "A datasource is unavailable, so its data could not be read. Try again.",
    ],
    [
      new ApiError(504, "query_timeout", "Slow"),
      "A datasource did not answer in time, so its data could not be read. Try again.",
    ],
    [
      new ApiError(422, "invalid_block", "markdown is required"),
      "Markdown is required.",
    ],
    [new Error("boom"), "The change could not be saved. Try again."],
  ])("explains %o in plain words", (error, expected) => {
    expect(addProblemText(error)).toBe(expected)
  })
})
