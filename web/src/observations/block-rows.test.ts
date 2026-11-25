import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"
import type { DataPanel } from "@/spec"

import {
  addProblemText,
  applyDraft,
  arrangeBody,
  blockHeight,
  blockRows,
  blockTitle,
  newPanelBody,
  dropTarget,
  holdsAgentOutput,
  moveBlock,
  followsLivePanel,
  nextBlockId,
  orderOf,
  packRows,
  rowGridClass,
  singleColumnBelow,
  spanClass,
  placeBeside,
  snapHeight,
  snapWidth,
  stepBlock,
  stepHeight,
  stepWidth,
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
    height: null,
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

  it("packs thirds, halves and two thirds over a row of three thirds", () => {
    expect(
      shape([
        block("a", 0, "third"),
        block("b", 1, "third"),
        block("c", 2, "third"),
        block("d", 3, "two_thirds"),
        block("e", 4, "third"),
        block("f", 5, "half"),
        block("g", 6, "half"),
      ])
    ).toEqual([
      ["a", "b", "c"],
      ["d", "e"],
      ["f", "g"],
    ])
  })

  it("starts a new row when the next block does not fit", () => {
    expect(
      shape([
        block("a", 0, "half"),
        block("b", 1, "two_thirds"),
        block("c", 2, "third"),
        block("d", 3, "two_thirds"),
      ])
    ).toEqual([["a"], ["b", "c"], ["d"]])
  })

  it("fits a third beside a half and leaves the rest of the row empty", () => {
    expect(
      shape([
        block("a", 0, "third"),
        block("b", 1, "half"),
        block("c", 2, "third"),
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
    expect(blockTitle(block("s", 0, "full", { kind: "snapshot" }))).toBe(
      "Snapshot"
    )
    expect(blockTitle(block("r", 0, "full", { kind: "run" }))).toBe("Run")
  })
})

describe("blockTitle of a text block", () => {
  const text = (markdown: unknown) =>
    blockTitle(block("t", 0, "full", { body: { markdown } }))

  it("names the block by the first words of its text", () => {
    expect(text("Checkout slowed after the deploy.")).toBe(
      "Checkout slowed after the deploy."
    )
    expect(
      text("The payment service started timing out at 10:42 after the deploy")
    ).toBe("The payment service started timing out…")
  })

  it("skips Markdown markers and blank lines", () => {
    expect(text("\n\n## **Root** cause\nmore")).toBe("Root cause")
    expect(text("- [the runbook](https://x.test/r) says restart")).toBe(
      "the runbook says restart"
    )
  })

  it("keeps snake_case words and strips paired emphasis", () => {
    expect(text("payment_service is *slow* and _late_")).toBe(
      "payment_service is slow and late"
    )
  })

  it("strips stacked block markers", () => {
    expect(text("> ## Root cause")).toBe("Root cause")
    expect(text("- 1. first step")).toBe("first step")
  })

  it("skips marker-only lines and thematic breaks", () => {
    expect(text("---\n***\n- - -\n##\n>\nReal text")).toBe("Real text")
    expect(text("---\n##")).toBe("Text")
  })

  it("allows one level of parentheses in a link address", () => {
    expect(text("[the page](https://x.test/a_(b)) is down")).toBe(
      "the page is down"
    )
  })

  it("clips by code points, not UTF-16 units", () => {
    const clipped = text("\u{1F600}".repeat(60))
    expect(Array.from(clipped)).toEqual([
      ...Array.from("\u{1F600}".repeat(48)),
      "\u2026",
    ])
  })

  it("cuts a long unbroken word", () => {
    expect(text("a".repeat(80))).toBe(`${"a".repeat(48)}…`)
  })

  it("falls back to Text when there is nothing to read", () => {
    expect(text("")).toBe("Text")
    expect(text("  \n ")).toBe("Text")
    expect(text(undefined)).toBe("Text")
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

describe("followsLivePanel", () => {
  const panel = (id: string, position: number, state: Block["state"]) =>
    block(id, position, "full", { kind: "panel", state })

  it("is true right after a live panel block in page order", () => {
    const blocks = [
      block("s", 1, "full", { state: "requested" }),
      panel("p", 0, "live"),
    ]
    expect(followsLivePanel(blocks, "s")).toBe(true)
  })

  it("is false after a text block, a requested panel or at the top", () => {
    expect(followsLivePanel([block("t", 0), block("s", 1)], "s")).toBe(false)
    expect(
      followsLivePanel([panel("p", 0, "requested"), block("s", 1)], "s")
    ).toBe(false)
    expect(followsLivePanel([panel("p", 1, "live"), block("s", 0)], "s")).toBe(
      false
    )
    expect(followsLivePanel([panel("p", 0, "live")], "zzz")).toBe(false)
  })
})

describe("holdsAgentOutput", () => {
  it("is true for a run block and for a block an agent wrote", () => {
    expect(holdsAgentOutput(block("a", 0, "full", { kind: "run" }))).toBe(true)
    expect(
      holdsAgentOutput(
        block("a", 0, "full", {
          created_by: { ...author, run_id: "r1", origin: "web" },
        })
      )
    ).toBe(true)
    expect(
      holdsAgentOutput(
        block("a", 0, "full", { created_by: { ...author, origin: "mcp" } })
      )
    ).toBe(true)
  })

  it("is false for a block a person wrote", () => {
    expect(holdsAgentOutput(block("a", 0))).toBe(false)
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

  it("strips the dashboard placement and carries the range it is given", () => {
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

describe("packRows", () => {
  it("packs any items that carry a width", () => {
    expect(
      packRows([{ width: "third" }, { width: "third" }, { width: "full" }]).map(
        (row) => row.length
      )
    ).toEqual([2, 1])
  })
})

describe("block heights", () => {
  it("uses the stored height, else the default of the width", () => {
    expect(blockHeight(block("a", 0, "half"))).toBe(220)
    expect(blockHeight(block("a", 0, "third"))).toBe(220)
    expect(blockHeight(block("a", 0, "two_thirds"))).toBe(280)
    expect(blockHeight(block("a", 0, "full"))).toBe(280)
    expect(blockHeight(block("a", 0, "full", { height: 400 }))).toBe(400)
  })

  it("snaps to 20 pixels and clamps to 120 to 960", () => {
    expect(snapHeight(251)).toBe(260)
    expect(snapHeight(10)).toBe(120)
    expect(snapHeight(5000)).toBe(960)
  })

  it("steps by 40 pixels and stops at the limits", () => {
    expect(stepHeight(280, 1)).toBe(320)
    expect(stepHeight(280, -1)).toBe(240)
    expect(stepHeight(130, -1)).toBe(120)
    expect(stepHeight(950, 1)).toBe(960)
  })
})

describe("snapWidth", () => {
  it("snaps a dragged width to the nearest of the four widths", () => {
    expect(snapWidth(300, 900)).toBe("third")
    expect(snapWidth(420, 900)).toBe("half")
    expect(snapWidth(560, 900)).toBe("two_thirds")
    expect(snapWidth(800, 900)).toBe("full")
    expect(snapWidth(2000, 900)).toBe("full")
    expect(snapWidth(1, 900)).toBe("third")
  })

  it("keeps full when the row has no width yet", () => {
    expect(snapWidth(300, 0)).toBe("full")
  })
})

describe("stepWidth", () => {
  it("moves one width at a time and stops at the ends", () => {
    expect(stepWidth("half", 1)).toBe("two_thirds")
    expect(stepWidth("half", -1)).toBe("third")
    expect(stepWidth("third", -1)).toBe("third")
    expect(stepWidth("full", 1)).toBe("full")
  })
})

describe("reordering", () => {
  const order = ["a", "b", "c", "d"]

  it("orders the blocks by position", () => {
    expect(orderOf([block("b", 1), block("a", 0)])).toEqual(["a", "b"])
  })

  it("moves a block to an index and clamps it", () => {
    expect(moveBlock(order, "d", 0)).toEqual(["d", "a", "b", "c"])
    expect(moveBlock(order, "a", 99)).toEqual(["b", "c", "d", "a"])
    expect(moveBlock(order, "a", -4)).toEqual(order)
    expect(moveBlock(order, "zzz", 1)).toEqual(order)
  })

  it("steps a block one place and stays at the ends", () => {
    expect(stepBlock(order, "b", -1)).toEqual(["b", "a", "c", "d"])
    expect(stepBlock(order, "b", 1)).toEqual(["a", "c", "b", "d"])
    expect(stepBlock(order, "a", -1)).toEqual(order)
    expect(stepBlock(order, "d", 1)).toEqual(order)
  })

  it("places a block before or after the one it is dropped on", () => {
    expect(placeBeside(order, "a", { id: "c", after: false })).toEqual([
      "b",
      "a",
      "c",
      "d",
    ])
    expect(placeBeside(order, "a", { id: "c", after: true })).toEqual([
      "b",
      "c",
      "a",
      "d",
    ])
    expect(placeBeside(order, "d", { id: "a", after: false })).toEqual([
      "d",
      "a",
      "b",
      "c",
    ])
    expect(placeBeside(order, "a", { id: "a", after: true })).toEqual(order)
    expect(placeBeside(order, "a", { id: "zzz", after: true })).toEqual(order)
  })
})

describe("dropTarget", () => {
  const rects = [
    { id: "a", left: 0, top: 0, width: 440, height: 200 },
    { id: "b", left: 460, top: 0, width: 440, height: 200 },
    { id: "c", left: 0, top: 220, width: 900, height: 200 },
  ]

  it("splits side by side blocks at their middle", () => {
    expect(dropTarget(rects, { x: 100, y: 50 }, "c", 900)).toEqual({
      id: "a",
      after: false,
      sideBySide: true,
    })
    expect(dropTarget(rects, { x: 800, y: 50 }, "c", 900)).toEqual({
      id: "b",
      after: true,
      sideBySide: true,
    })
  })

  it("splits a full width block at its vertical middle", () => {
    expect(dropTarget(rects, { x: 100, y: 230 }, "a", 900)).toEqual({
      id: "c",
      after: false,
      sideBySide: false,
    })
    expect(dropTarget(rects, { x: 100, y: 400 }, "a", 900)).toEqual({
      id: "c",
      after: true,
      sideBySide: false,
    })
  })

  it("names nothing over the dragged block, a gap or empty space", () => {
    expect(dropTarget(rects, { x: 100, y: 50 }, "a", 900)).toBeNull()
    expect(dropTarget(rects, { x: 450, y: 50 }, "c", 900)).toBeNull()
    expect(dropTarget(rects, { x: 100, y: 900 }, "c", 900)).toBeNull()
  })
})

describe("a draft layout", () => {
  const blocks = [
    block("a", 0, "half"),
    block("b", 1, "half", { height: 300 }),
    block("c", 2, "full"),
  ]

  it("applies the order, widths and heights, and renumbers positions", () => {
    const draft = {
      order: ["c", "a", "b"],
      changes: {
        a: { width: "third" as const, height: 400 },
        b: { height: null },
      },
    }
    const arranged = applyDraft(blocks, draft)
    expect(arranged.map((b) => [b.id, b.position, b.width, b.height])).toEqual([
      ["c", 0, "full", null],
      ["a", 1, "third", 400],
      ["b", 2, "half", null],
    ])
  })

  it("keeps blocks the draft does not name, and skips unknown ids", () => {
    const arranged = applyDraft(blocks, { order: ["zzz", "c"], changes: {} })
    expect(arranged.map((b) => b.id)).toEqual(["c", "a", "b"])
  })

  it("sends the whole order but only the changes it holds", () => {
    expect(
      arrangeBody({
        order: ["b", "a"],
        changes: { b: { width: "third", height: null } },
      })
    ).toEqual({
      blocks: [{ id: "b", width: "third", height: null }, { id: "a" }],
    })
  })
})

describe("the width below which the page shows one column", () => {
  const publishedColumnPixels = 720 - 2 * 16
  const phonePixels = 430

  it("is narrower than the published page's content and wider than a phone", () => {
    expect(singleColumnBelow).toBeLessThan(publishedColumnPixels)
    expect(singleColumnBelow).toBeGreaterThan(phonePixels)
  })

  it("is the width every row class switches at", () => {
    const classes = [rowGridClass, ...Object.values(spanClass)]
    for (const name of classes) {
      expect(name).toContain(`@min-[${singleColumnBelow}px]:`)
    }
  })
})
