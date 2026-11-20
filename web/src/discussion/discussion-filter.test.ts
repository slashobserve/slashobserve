import { describe, expect, it } from "vitest"

import type { Message } from "@/comments/thread-queries"

import {
  filterEntries,
  isFiltered,
  noFilter,
  type DiscussionFilter,
} from "./discussion-filter"
import { discussionEntries, type DiscussionChange } from "./discussion-model"

const ada = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function stamp(minute: number): string {
  return `2026-11-10T09:${String(minute).padStart(2, "0")}:00Z`
}

function message(
  id: string,
  minute: number,
  overrides: Partial<Message> = {}
): Message {
  return {
    id,
    thread_id: "m1",
    body: id,
    actor: ada,
    created_at: stamp(minute),
    updated_at: stamp(minute),
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
    ...overrides,
  }
}

function quoting(id: string) {
  return { id, actor: ada, excerpt: id }
}

function change(
  key: string,
  minute: number,
  subject: DiscussionChange["subject"]
): DiscussionChange {
  return {
    key,
    at: stamp(minute),
    actor: ada,
    verb: "added a block",
    subject,
    detail: null,
  }
}

const messages = [
  message("m1", 1),
  message("m2", 2, { block_id: "b1" }),
  message("m3", 3, { block_id: "b2" }),
  message("m4", 4, { reply_to: quoting("m2") }),
  message("m5", 5, { reply_to: quoting("m4") }),
  message("m6", 6, { reply_to: quoting("m3") }),
]
const changes = [
  change("started", 0, null),
  change("b1", 1, { id: "b1", title: "Text" }),
  change("b2", 2, { id: "b2", title: "Panel: Latency" }),
]
const entries = discussionEntries(messages, changes)

function keys(filter: DiscussionFilter): string[] {
  return filterEntries(entries, filter).map((entry) => entry.key)
}

describe("filterEntries", () => {
  it("keeps everything when nothing is chosen", () => {
    expect(filterEntries(entries, noFilter)).toEqual(entries)
  })

  it("keeps only messages or only changes by what is shown", () => {
    expect(keys({ show: "messages", blockId: null })).toEqual([
      "message:m1",
      "message:m2",
      "message:m3",
      "message:m4",
      "message:m5",
      "message:m6",
    ])
    expect(keys({ show: "changes", blockId: null })).toEqual([
      "change:started",
      "change:b1",
      "change:b2",
    ])
  })

  it("keeps a block's messages, its changes and the replies that quote them", () => {
    expect(keys({ show: "everything", blockId: "b1" })).toEqual([
      "change:b1",
      "message:m2",
      "message:m4",
      "message:m5",
    ])
  })

  it("applies the block to what is shown", () => {
    expect(keys({ show: "messages", blockId: "b2" })).toEqual([
      "message:m3",
      "message:m6",
    ])
    expect(keys({ show: "changes", blockId: "b2" })).toEqual(["change:b2"])
  })

  it("drops a reply whose quoted message is not kept", () => {
    const orphan = discussionEntries(
      [message("m9", 9, { reply_to: quoting("gone") })],
      []
    )
    expect(
      filterEntries(orphan, { show: "everything", blockId: "b1" })
    ).toEqual([])
  })

  it("matches nothing for a block with no entries", () => {
    expect(keys({ show: "everything", blockId: "b9" })).toEqual([])
  })
})

describe("isFiltered", () => {
  it("is false only for everything on all blocks", () => {
    expect(isFiltered(noFilter)).toBe(false)
    expect(isFiltered({ show: "messages", blockId: null })).toBe(true)
    expect(isFiltered({ show: "everything", blockId: "b1" })).toBe(true)
  })
})
