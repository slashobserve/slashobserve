import { describe, expect, it } from "vitest"

import {
  idle,
  placements,
  settle,
  want,
  type Placement,
  type Queue,
} from "./arrange-queue"
import type { Block } from "./queries"

const full = (id: string): Placement => ({ id, width: "full", height: null })

function layout(...ids: string[]): Placement[] {
  return ids.map(full)
}

function wanted(...ids: string[]) {
  return { layout: layout(...ids), body: ids.join("") }
}

const cache = layout("a", "b", "c")

describe("the arrange write queue", () => {
  it("sends the first change and nothing when the layout is already the cache", () => {
    expect(want(idle<string>(), wanted("a", "b", "c"), cache)).toEqual({
      queue: idle<string>(),
      send: null,
    })
    const first = want(idle<string>(), wanted("b", "a", "c"), cache)
    expect(first.send).toEqual(wanted("b", "a", "c"))
    expect(first.queue.inFlight).toEqual(wanted("b", "a", "c"))
  })

  it("keeps at most one write in flight and sends the latest wanted layout when it returns", () => {
    let queue: Queue<string> = want(
      idle<string>(),
      wanted("b", "a", "c"),
      cache
    ).queue
    for (const next of [wanted("b", "c", "a"), wanted("c", "b", "a")]) {
      const result = want(queue, next, cache)
      expect(result.send).toBeNull()
      queue = result.queue
    }
    const done = settle(queue, true)
    expect(done.send).toEqual(wanted("c", "b", "a"))
    expect(done.queue.inFlight).toEqual(wanted("c", "b", "a"))
    expect(done.queue.wanted).toBeNull()
    expect(settle(done.queue, true)).toEqual({
      queue: idle<string>(),
      send: null,
    })
  })

  it("sends the undone layout after the pending write instead of dropping it", () => {
    const first = want(idle<string>(), wanted("b", "a", "c"), cache)
    const undone = want(first.queue, wanted("a", "b", "c"), cache)
    expect(undone.send).toBeNull()
    expect(settle(undone.queue, true).send).toEqual(wanted("a", "b", "c"))
  })

  it("sends nothing when the wanted layout is the one already in flight", () => {
    const first = want(idle<string>(), wanted("b", "a", "c"), cache)
    const there = want(first.queue, wanted("a", "b", "c"), cache)
    const back = want(there.queue, wanted("b", "a", "c"), cache)
    expect(back.queue.wanted).toBeNull()
    expect(settle(back.queue, true)).toEqual({
      queue: idle<string>(),
      send: null,
    })
  })

  it("drops what is waiting when the write in flight fails", () => {
    const first = want(idle<string>(), wanted("b", "a", "c"), cache)
    const next = want(first.queue, wanted("c", "a", "b"), cache)
    expect(settle(next.queue, false)).toEqual({
      queue: idle<string>(),
      send: null,
    })
  })

  it("tells widths and heights apart", () => {
    const widened = [{ ...full("a"), width: "half" as const }, full("b")]
    const taller = [{ ...full("a"), height: 400 }, full("b")]
    const first = want(
      idle<string>(),
      { layout: widened, body: "w" },
      layout("a", "b")
    )
    const stepped = want(
      first.queue,
      { layout: layout("a", "b"), body: "n" },
      layout("a", "b")
    )
    expect(settle(stepped.queue, true).send).toEqual({
      layout: layout("a", "b"),
      body: "n",
    })
    expect(
      want(idle<string>(), { layout: taller, body: "h" }, layout("a", "b")).send
    ).not.toBeNull()
  })

  it("reads placements from blocks in position order", () => {
    const block = (id: string, position: number, extra: Partial<Block> = {}) =>
      ({ id, position, width: "full", height: null, ...extra }) as Block
    expect(
      placements([
        block("b", 1, { height: 300 }),
        block("a", 0, { width: "half" }),
      ])
    ).toEqual([
      { id: "a", width: "half", height: null },
      { id: "b", width: "full", height: 300 },
    ])
  })
})
