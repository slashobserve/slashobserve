import { threadVerbText } from "@/activity/thread-view"
import type { components } from "@/api/schema.gen"
import type { Message, ThreadWithMessages } from "@/comments/thread-queries"

type Actor = components["schemas"]["Actor"]

export type DiscussionShow = "everything" | "messages" | "changes"

export type DiscussionChange = {
  key: string
  at: string
  actor: Actor
  verb: string
  subject: { id: string; title: string } | null
}

export type DiscussionEntry =
  | { kind: "message"; key: string; at: string; message: Message; verb: string }
  | { kind: "change"; key: string; at: string; change: DiscussionChange }

export type DiscussionModel = {
  messages: Message[]
  replyThreadId: string | null
  canWrite: boolean
}

function byCreation(
  a: { created_at: string; id: string },
  b: { created_at: string; id: string }
): number {
  const gap = Date.parse(a.created_at) - Date.parse(b.created_at)
  return gap !== 0 ? gap : a.id.localeCompare(b.id)
}

function replyTarget(threads: readonly ThreadWithMessages[]): string | null {
  let newest: { created_at: string; id: string } | null = null
  let threadId: string | null = null
  for (const { thread, messages } of threads) {
    if (!thread.allowed_actions.includes("reply")) continue
    for (const message of messages) {
      if (newest === null || byCreation(message, newest) > 0) {
        newest = message
        threadId = thread.id
      }
    }
  }
  return threadId
}

export function discussionModel(
  threads: readonly ThreadWithMessages[],
  canComment: boolean
): DiscussionModel {
  const messages = threads.flatMap((entry) => entry.messages).sort(byCreation)
  const replyThreadId = replyTarget(threads)
  return {
    messages,
    replyThreadId,
    canWrite: threads.length === 0 ? canComment : replyThreadId !== null,
  }
}

export function messageVerb(message: Message): string {
  if (message.id !== message.thread_id) return "replied"
  const asked = message.mentions.some((mention) => mention.kind !== "user")
  return threadVerbText(asked, null, null)
}

function byTime(a: DiscussionEntry, b: DiscussionEntry): number {
  const gap = Date.parse(a.at) - Date.parse(b.at)
  if (gap !== 0) return gap
  if (a.kind !== b.kind) return a.kind === "change" ? -1 : 1
  return a.key.localeCompare(b.key)
}

export function discussionEntries(
  messages: readonly Message[],
  changes: readonly DiscussionChange[],
  show: DiscussionShow
): DiscussionEntry[] {
  const entries: DiscussionEntry[] = []
  if (show !== "changes") {
    for (const message of messages) {
      entries.push({
        kind: "message",
        key: `message:${message.id}`,
        at: message.created_at,
        message,
        verb: messageVerb(message),
      })
    }
  }
  if (show !== "messages") {
    for (const change of changes) {
      entries.push({
        kind: "change",
        key: `change:${change.key}`,
        at: change.at,
        change,
      })
    }
  }
  return entries.sort(byTime)
}

export function messageCount(threads: readonly ThreadWithMessages[]): number {
  return threads.reduce((total, entry) => total + entry.messages.length, 0)
}
