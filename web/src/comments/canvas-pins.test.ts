import { describe, expect, it } from "vitest"

import {
  markerStyle,
  panelPins,
  pinLabel,
  stackLabel,
  stackPins,
  type PanelPin,
} from "./canvas-pins"
import { sitsOn } from "./canvas-host"
import type { Message, Thread, ThreadWithMessages } from "./thread-queries"

const actor: Message["actor"] = {
  kind: "user",
  id: "u1",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

function entry(
  id: string,
  thread: Partial<Thread>,
  messages = 1
): ThreadWithMessages {
  return {
    thread: {
      id,
      panel_id: "p1",
      status: "open",
      anchor: { kind: "position", pos: { x: 0.5, y: 0.5 } },
      ...thread,
    } as Thread,
    messages: Array.from({ length: messages }, (_, index) => ({
      id: `${id}-m${index}`,
      body: "Errors **doubled** after the deploy",
      actor,
    })) as Message[],
  }
}

const here = { at: { x: 10, y: 20 }, gone: false }

describe("panelPins", () => {
  it("keeps anchored threads on this panel that have a message", () => {
    const pins = panelPins(
      [
        entry("t1", {}),
        entry("t2", { panel_id: "p2" }),
        entry("t3", { anchor: null }),
        entry("t4", {}, 0),
      ],
      (thread) => sitsOn({ kind: "dashboard", id: "d1" }, "p1", thread),
      () => here
    )
    expect(pins.map((pin) => pin.entry.thread.id)).toEqual(["t1"])
    expect(pins[0].placement).toBe(here)
  })

  it("keeps the threads on one observation block by their block", () => {
    const pins = panelPins(
      [
        entry("t1", { panel_id: null, block_id: "b1" }),
        entry("t2", { panel_id: null, block_id: "b2" }),
        entry("t3", { panel_id: "b1", block_id: null }),
      ],
      (thread) => sitsOn({ kind: "observation", id: "o1" }, "b1", thread),
      () => here
    )
    expect(pins.map((pin) => pin.entry.thread.id)).toEqual(["t1"])
  })
})

describe("pinLabel", () => {
  function pin(thread: Partial<Thread>, gone = false): PanelPin {
    const shown = entry("t1", thread)
    return {
      entry: shown,
      anchor: shown.thread.anchor!,
      placement: { ...here, gone },
    }
  }

  it("names the author and the start of the comment", () => {
    expect(pinLabel(pin({}))).toBe(
      "Comment by Ada: Errors doubled after the deploy"
    )
  })

  it("says when the thread is resolved and when its data is gone", () => {
    expect(pinLabel(pin({ status: "resolved" }, true))).toBe(
      "Comment by Ada: Errors doubled after the deploy, resolved, its data is no longer on the panel"
    )
  })

  it("says a text comment is detached once its text is gone", () => {
    const anchor = { kind: "text" as const, pos: { x: 0, y: 0 }, quote: "x" }
    expect(pinLabel(pin({ anchor, block_id: "b1" }, true))).toBe(
      "Comment by Ada: Errors doubled after the deploy, its text is no longer in the block"
    )
  })
})

describe("stackPins", () => {
  function at(id: string, x: number, y: number, status = "open"): PanelPin {
    const shown = entry(id, { status: status as Thread["status"] })
    return {
      entry: shown,
      anchor: shown.thread.anchor!,
      placement: { at: { x, y }, gone: false },
    }
  }

  it("stacks pins that would overlap and keeps the others apart", () => {
    const stacks = stackPins([
      at("t1", 100, 100),
      at("t2", 110, 105),
      at("t3", 200, 100),
      at("t4", 95, 90),
    ])
    expect(
      stacks.map(({ at, pins }) => [at, pins.map((pin) => pin.entry.thread.id)])
    ).toEqual([
      [{ x: 100, y: 100 }, ["t1", "t2", "t4"]],
      [{ x: 200, y: 100 }, ["t3"]],
    ])
  })

  it("counts the comments and the resolved ones", () => {
    const [stack] = stackPins([
      at("t1", 0, 0),
      at("t2", 1, 1, "resolved"),
      at("t3", 2, 2),
    ])
    expect(stackLabel(stack)).toBe("3 comments here, 1 resolved")
    const [open] = stackPins([at("t1", 0, 0), at("t2", 1, 1)])
    expect(stackLabel(open)).toBe("2 comments here")
  })
})

describe("markerStyle", () => {
  it("keeps the marker clear of the point it points at", () => {
    const at = { x: 100, y: 60 }
    const { left, top, width, height } = markerStyle(at)
    const clearOfPoint = left > at.x || top + height < at.y
    expect(clearOfPoint).toBe(true)
    expect(width).toBe(height)
  })
})
