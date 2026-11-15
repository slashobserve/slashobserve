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
  }
}

function entry(
  id: string,
  allowed: Thread["allowed_actions"],
  messages: Message[]
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
      created_at: messages[0]?.created_at ?? "2026-11-10T09:00:00Z",
      updated_at: "2026-11-10T09:00:00Z",
      resolved_at: null,
      resolved_actor: null,
      resolved_by_version: null,
      revision: null,
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
      expect(discussionModel([], canComment)).toEqual({
        messages: [],
        replyThreadId: null,
        canWrite,
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
    const model = discussionModel([first, second], true)
    expect(model.messages.map((m) => m.id)).toEqual(["m1", "m2", "m3", "m4"])
  })

  it("replies in the thread with the newest message", () => {
    const older = entry(
      "t1",
      ["reply", "resolve"],
      [message("m1", "t1", "2026-11-10T09:00:00Z")]
    )
    const newer = entry(
      "t2",
      ["reply"],
      [message("m2", "t2", "2026-11-10T10:00:00Z")]
    )
    expect(discussionModel([older, newer], false)).toMatchObject({
      replyThreadId: "t2",
      canWrite: true,
    })
    expect(discussionModel([newer, older], false).replyThreadId).toBe("t2")
  })

  it("never replies in a thread that does not allow it", () => {
    const closed = entry(
      "t2",
      ["reopen"],
      [message("m2", "t2", "2026-11-10T10:00:00Z")]
    )
    const open = entry(
      "t1",
      ["reply"],
      [message("m1", "t1", "2026-11-10T09:00:00Z")]
    )
    expect(discussionModel([open, closed], true).replyThreadId).toBe("t1")
    expect(discussionModel([closed], true)).toMatchObject({
      replyThreadId: null,
      canWrite: false,
    })
  })
})

function change(key: string, at: string): DiscussionChange {
  return { key, at, actor: ada, verb: "added a block", subject: null }
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
    expect(
      discussionEntries(messages, changes, "everything").map((e) => e.key)
    ).toEqual(["change:started", "message:m1", "change:b1", "message:m2"])
  })

  it("puts a change before a message at the same instant", () => {
    const tied = [change("b2", "2026-11-10T09:00:00Z")]
    expect(
      discussionEntries(messages, tied, "everything").map((e) => e.key)
    ).toEqual(["change:b2", "message:m1", "message:m2"])
  })

  it("orders changes at the same instant by key", () => {
    const tied = [
      change("b", "2026-11-10T09:00:00Z"),
      change("a", "2026-11-10T09:00:00Z"),
    ]
    expect(discussionEntries([], tied, "everything").map((e) => e.key)).toEqual(
      ["change:a", "change:b"]
    )
  })

  it("keeps only messages or only changes when filtered", () => {
    expect(
      discussionEntries(messages, changes, "messages").map((e) => e.key)
    ).toEqual(["message:m1", "message:m2"])
    expect(
      discussionEntries(messages, changes, "changes").map((e) => e.key)
    ).toEqual(["change:started", "change:b1"])
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
    "says asked the agent for a first message mentioning %s",
    (kind) => {
      expect(
        messageVerb({
          ...message("m1", "m1", "2026-11-10T09:00:00Z"),
          mentions: [mention(kind)],
        })
      ).toBe("asked the agent")
    }
  )

  it("says replied for a later message", () => {
    expect(messageVerb(message("m2", "m1", "2026-11-10T09:00:00Z"))).toBe(
      "replied"
    )
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
