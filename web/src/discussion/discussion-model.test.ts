import { describe, expect, it } from "vitest"

import type {
  Message,
  Thread,
  ThreadWithMessages,
} from "@/comments/thread-queries"

import {
  discussionEntries,
  discussionModel,
  messageCount,
  messageVerb,
  threadAllowsReply,
  quotedText,
  type DiscussionChange,
} from "./discussion-model"

const ada = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function message(id: string, threadId: string, createdAt: string): Message {
  return {
    id,
    thread_id: threadId,
    body: id,
    actor: ada,
    created_at: createdAt,
    updated_at: createdAt,
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
  }
}

function entry(
  id: string,
  allowed: Thread["allowed_actions"],
  messages: Message[],
  spot: Pick<Thread, "anchor" | "block_id"> = { anchor: null, block_id: null }
): ThreadWithMessages {
  return {
    thread: {
      id,
      target: { kind: "observation", id: "f1" },
      dashboard_id: null,
      version: null,
      panel_id: null,
      panel_present: true,
      status: "open",
      allowed_actions: allowed,
      unread: false,
      created_at: messages[0]?.created_at ?? "2026-11-10T09:00:00Z",
      updated_at: "2026-11-10T09:00:00Z",
      resolved_at: null,
      resolved_actor: null,
      resolved_by_version: null,
      revision: null,
      ...spot,
      agent: null,
      agent_off: false,
    },
    messages,
  }
}

describe("discussionModel", () => {
  it.each([
    { canComment: true, canWrite: true },
    { canComment: false, canWrite: false },
  ])(
    "with no thread, writing is allowed only when the caller may comment ($canComment)",
    ({ canComment, canWrite }) => {
      expect(discussionModel([], canComment, null)).toEqual({
        messages: [],
        posting: { thread: null, canWrite },
      })
    }
  )

  it("interleaves two threads by time and breaks ties by id", () => {
    const first = entry(
      "t1",
      ["reply"],
      [
        message("m1", "t1", "2026-11-10T09:00:00Z"),
        message("m4", "t1", "2026-11-10T09:30:00Z"),
      ]
    )
    const second = entry(
      "t2",
      ["reply"],
      [
        message("m3", "t2", "2026-11-10T09:30:00Z"),
        message("m2", "t2", "2026-11-10T09:10:00Z"),
      ]
    )
    const model = discussionModel([first, second], true, null)
    expect(model.messages.map((m) => m.id)).toEqual(["m1", "m2", "m3", "m4"])
  })
})

const timeAnchor = { kind: "title" as const, pos: { x: 0.1, y: 0.2 } }

describe("discussionModel posting", () => {
  const page = entry(
    "page",
    ["reply"],
    [message("m1", "page", "2026-11-10T09:00:00Z")]
  )
  const block = entry(
    "block",
    ["reply"],
    [message("m2", "block", "2026-11-10T10:00:00Z")],
    { anchor: timeAnchor, block_id: "b1" }
  )
  const resolvedPage = entry(
    "page",
    ["reopen"],
    [message("m1", "page", "2026-11-10T09:00:00Z")]
  )
  const resolvedBlock = entry(
    "block",
    ["reopen"],
    [message("m2", "block", "2026-11-10T10:00:00Z")],
    { anchor: timeAnchor, block_id: "b1" }
  )
  const postingOf = (
    threads: ThreadWithMessages[],
    canComment: boolean,
    replyingTo: Pick<Message, "thread_id"> | null = null
  ) => {
    const { posting } = discussionModel(threads, canComment, replyingTo)
    return { id: posting.thread?.thread.id ?? null, canWrite: posting.canWrite }
  }

  it("posts a whole-page message to the page thread, not the newest thread", () => {
    expect(postingOf([page, block], false)).toEqual({
      id: "page",
      canWrite: true,
    })
    expect(postingOf([block, page], false)).toEqual({
      id: "page",
      canWrite: true,
    })
  })

  it("opens a page thread when only block threads exist, if the caller may comment", () => {
    expect(postingOf([block], true)).toEqual({ id: null, canWrite: true })
    expect(postingOf([block], false)).toEqual({ id: null, canWrite: false })
  })

  it("opens a fresh page thread when every thread is resolved but the caller may comment", () => {
    expect(postingOf([resolvedBlock], true)).toEqual({
      id: null,
      canWrite: true,
    })
  })

  it("cannot write whole-page when the page thread exists and is resolved", () => {
    expect(postingOf([resolvedPage, block], true)).toEqual({
      id: null,
      canWrite: false,
    })
  })

  it("replies in the thread of the message that was pressed, even an older one", () => {
    expect(postingOf([page, block], false, { thread_id: "page" }).id).toBe(
      "page"
    )
    expect(postingOf([page, block], false, { thread_id: "block" })).toEqual({
      id: "block",
      canWrite: true,
    })
  })

  it("ignores a reply target whose thread does not allow a reply", () => {
    expect(
      postingOf([page, resolvedBlock], true, { thread_id: "block" })
    ).toEqual({ id: "page", canWrite: true })
    expect(postingOf([page], true, { thread_id: "gone" }).id).toBe("page")
  })
})

describe("threadAllowsReply", () => {
  const open = entry("t1", ["reply"], [])
  const closed = entry("t2", ["reopen"], [])

  it("is true only for a loaded thread that allows a reply", () => {
    expect(threadAllowsReply([open, closed], "t1")).toBe(true)
    expect(threadAllowsReply([open, closed], "t2")).toBe(false)
    expect(threadAllowsReply([open, closed], "gone")).toBe(false)
  })
})

function change(key: string, at: string): DiscussionChange {
  return {
    key,
    at,
    actor: ada,
    verb: "added a block",
    subject: null,
    detail: null,
  }
}

describe("discussionEntries", () => {
  const messages = [
    message("m1", "m1", "2026-11-10T09:00:00Z"),
    message("m2", "m1", "2026-11-10T09:30:00Z"),
  ]
  const changes = [
    change("started", "2026-11-10T08:00:00Z"),
    change("b1", "2026-11-10T09:10:00Z"),
  ]

  it("interleaves messages and changes by time", () => {
    expect(discussionEntries(messages, changes).map((e) => e.key)).toEqual([
      "change:started",
      "message:m1",
      "change:b1",
      "message:m2",
    ])
  })

  it("puts a change before a message at the same instant", () => {
    const tied = [change("b2", "2026-11-10T09:00:00Z")]
    expect(discussionEntries(messages, tied).map((e) => e.key)).toEqual([
      "change:b2",
      "message:m1",
      "message:m2",
    ])
  })

  it("orders changes at the same instant by key", () => {
    const tied = [
      change("b", "2026-11-10T09:00:00Z"),
      change("a", "2026-11-10T09:00:00Z"),
    ]
    expect(discussionEntries([], tied).map((e) => e.key)).toEqual([
      "change:a",
      "change:b",
    ])
  })
})

describe("messageVerb", () => {
  const mention = (kind: "user" | "agent" | "grant") => ({
    kind,
    id: "x",
    name: "x",
  })

  it("says commented for a first message that asks nobody", () => {
    expect(messageVerb(message("m1", "m1", "2026-11-10T09:00:00Z"))).toBe(
      "commented"
    )
    expect(
      messageVerb({
        ...message("m1", "m1", "2026-11-10T09:00:00Z"),
        mentions: [mention("user")],
      })
    ).toBe("commented")
  })

  it.each(["agent", "grant"] as const)(
    "says asked the agent for a first message mentioning %s that started a run",
    (kind) => {
      expect(
        messageVerb({
          ...message("m1", "m1", "2026-11-10T09:00:00Z"),
          mentions: [mention(kind)],
          run: { id: "r1", status: "queued", assigned_kind: "builtin" },
        })
      ).toBe("asked the agent")
    }
  )

  it.each(["agent", "grant"] as const)(
    "says mentioned the agent when the first message mentioning %s started no run",
    (kind) => {
      expect(
        messageVerb({
          ...message("m1", "m1", "2026-11-10T09:00:00Z"),
          mentions: [mention(kind)],
          run: null,
        })
      ).toBe("mentioned the agent")
    }
  )

  it("says replied for a later message", () => {
    expect(messageVerb(message("m2", "m1", "2026-11-10T09:00:00Z"))).toBe(
      "replied"
    )
  })
})

describe("messageVerb on a quoting reply", () => {
  it("names the author of the quoted message", () => {
    const quoting = {
      ...message("m2", "m1", "2026-11-10T09:00:00Z"),
      reply_to: {
        id: "m1",
        actor: { ...ada, label: "Observe Agent" },
        excerpt: "p95 rose",
      },
    }
    expect(messageVerb(quoting)).toBe("replied to Observe Agent")
  })
})

describe("quotedText", () => {
  it.each([
    ["p95 rose after the deploy", "p95 rose after the deploy"],
    [
      "@[Grace](user:2f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) look at this",
      "@Grace look at this",
    ],
    ["@observe why", "@observe why"],
    ["cut mid token @[Gra", "cut mid token @[Gra"],
  ])("shows %j as %j", (excerpt, text) => {
    expect(quotedText(excerpt)).toBe(text)
  })
})

describe("messageCount", () => {
  it("totals the messages of every thread", () => {
    const one = entry(
      "t1",
      ["reply"],
      [message("m1", "t1", "2026-11-10T09:00:00Z")]
    )
    const two = entry(
      "t2",
      ["reply"],
      [
        message("m2", "t2", "2026-11-10T09:00:00Z"),
        message("m3", "t2", "2026-11-10T09:01:00Z"),
      ]
    )
    expect(messageCount([])).toBe(0)
    expect(messageCount([one])).toBe(1)
    expect(messageCount([one, two])).toBe(3)
  })
})
