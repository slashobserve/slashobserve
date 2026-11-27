import { describe, expect, it } from "vitest"

import { applyInboxChange, unreadAfter, type InboxPages } from "./inbox-cache"
import type { InboxItem } from "./inbox-item"

function item(
  seq: number,
  unread: boolean,
  reason: InboxItem["reason"] = "mention"
): InboxItem {
  return {
    seq,
    unread,
    reason,
    state: unread ? "unread" : "read",
  } as InboxItem
}

function list(...pages: InboxItem[][]): InboxPages {
  return {
    pages: pages.map((items) => ({
      items,
      next_cursor: null,
      unread_count: 3,
    })),
    pageParams: pages.map(() => undefined),
  }
}

const forYou = list([item(9, true), item(8, true)], [item(5, true)])
const everything = list([item(10, true), item(9, true, null), item(7, false)])

describe("unreadAfter", () => {
  it("drops to the unread items newer than a read through", () => {
    expect(unreadAfter(3, [forYou], { kind: "readThrough", seq: 9 })).toBe(0)
    expect(unreadAfter(3, [forYou], { kind: "readThrough", seq: 8 })).toBe(1)
  })

  it("counts only items with a reason after a read through", () => {
    expect(
      unreadAfter(3, [forYou, everything], { kind: "readThrough", seq: 8 })
    ).toBe(2)
  })

  it("never raises the count on a read through", () => {
    expect(
      unreadAfter(1, [forYou, everything], { kind: "readThrough", seq: 4 })
    ).toBe(1)
  })

  it("moves by one when an item for you is marked read, unread or done", () => {
    expect(
      unreadAfter(3, [forYou], { kind: "unread", seq: 8, unread: false })
    ).toBe(2)
    expect(
      unreadAfter(2, [everything], { kind: "unread", seq: 7, unread: true })
    ).toBe(3)
    expect(unreadAfter(3, [forYou], { kind: "done", seq: 5, done: true })).toBe(
      2
    )
  })

  it("keeps the count for an item that is not for you, not loaded or already read", () => {
    expect(
      unreadAfter(3, [everything], { kind: "unread", seq: 9, unread: false })
    ).toBe(3)
    expect(
      unreadAfter(3, [forYou], { kind: "unread", seq: 99, unread: false })
    ).toBe(3)
    expect(
      unreadAfter(3, [everything], { kind: "done", seq: 7, done: true })
    ).toBe(3)
    expect(
      unreadAfter(3, [forYou], { kind: "done", seq: 8, done: false })
    ).toBe(3)
  })

  it("keeps 100 or more at the cap when one item is read, and never passes it", () => {
    expect(
      unreadAfter(100, [forYou], { kind: "unread", seq: 8, unread: false })
    ).toBe(100)
    expect(
      unreadAfter(100, [everything], { kind: "unread", seq: 7, unread: true })
    ).toBe(100)
  })
})

describe("applyInboxChange", () => {
  it("marks every loaded item up to the seq read and sets the count on each page", () => {
    const next = applyInboxChange(forYou, { kind: "readThrough", seq: 8 }, 1)
    expect(next.pages.map((page) => page.items.map((i) => i.unread))).toEqual([
      [true, false],
      [false],
    ])
    expect(next.pages.map((page) => page.unread_count)).toEqual([1, 1])
  })

  it("changes only the item named, and reads it when done", () => {
    const read = applyInboxChange(
      forYou,
      { kind: "unread", seq: 9, unread: false },
      2
    )
    expect(read.pages[0].items.map((i) => i.unread)).toEqual([false, true])
    const done = applyInboxChange(
      forYou,
      { kind: "done", seq: 5, done: true },
      2
    )
    expect(done.pages[1].items[0].unread).toBe(false)
    expect(forYou.pages[1].items[0].unread).toBe(true)
  })

  it("keeps each item's state in step with the change", () => {
    const states = (pages: InboxPages) =>
      pages.pages.flatMap((page) => page.items.map((i) => i.state))
    expect(
      states(applyInboxChange(forYou, { kind: "readThrough", seq: 8 }, 1))
    ).toEqual(["unread", "read", "read"])
    expect(
      states(
        applyInboxChange(forYou, { kind: "unread", seq: 9, unread: false }, 2)
      )
    ).toEqual(["read", "unread", "unread"])
    const done = applyInboxChange(
      forYou,
      { kind: "done", seq: 5, done: true },
      2
    )
    expect(states(done)).toEqual(["unread", "unread", "done"])
    expect(
      states(applyInboxChange(done, { kind: "readThrough", seq: 9 }, 0))
    ).toEqual(["read", "read", "done"])
    expect(
      states(
        applyInboxChange(done, { kind: "unread", seq: 5, unread: false }, 2)
      )
    ).toEqual(["unread", "unread", "done"])
    expect(
      states(applyInboxChange(done, { kind: "done", seq: 5, done: false }, 2))
    ).toEqual(["unread", "unread", "read"])
    expect(
      states(
        applyInboxChange(done, { kind: "unread", seq: 5, unread: true }, 3)
      )
    ).toEqual(["unread", "unread", "unread"])
  })
})
