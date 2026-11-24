import type { components } from "@/api/schema.gen"
import { mentionSegments } from "@/comments/mentions"
import type { Message, ThreadWithMessages } from "@/comments/thread-queries"

type Actor = components["schemas"]["Actor"]

export type DiscussionShow = "everything" | "messages" | "changes"

export type DiscussionChange = {
  key: string
  at: string
  actor: Actor
  verb: string
  subject: { id: string; title: string } | null
  detail: string | null
}

export type DiscussionEntry =
  | { kind: "message"; key: string; at: string; message: Message; verb: string }
  | { kind: "change"; key: string; at: string; change: DiscussionChange }

function byCreation(
  a: { created_at: string; id: string },
  b: { created_at: string; id: string }
): number {
  const gap = Date.parse(a.created_at) - Date.parse(b.created_at)
  return gap !== 0 ? gap : a.id.localeCompare(b.id)
}

type Posting = { thread: ThreadWithMessages | null; canWrite: boolean }

export type DiscussionModel = {
  messages: Message[]
  posting: Posting
}

export function threadAllowsReply(
  threads: readonly ThreadWithMessages[],
  threadId: string
): boolean {
  return threads.some(
    ({ thread }) =>
      thread.id === threadId && thread.allowed_actions.includes("reply")
  )
}

function postingOf(
  threads: readonly ThreadWithMessages[],
  canComment: boolean,
  replyingTo: Pick<Message, "thread_id"> | null
): Posting {
  if (replyingTo !== null) {
    const quoted = threads.find(
      ({ thread }) =>
        thread.id === replyingTo.thread_id &&
        thread.allowed_actions.includes("reply")
    )
    if (quoted !== undefined) return { thread: quoted, canWrite: true }
  }
  const page = threads.find(
    ({ thread }) => thread.anchor === null && thread.block_id === null
  )
  if (page === undefined) return { thread: null, canWrite: canComment }
  return page.thread.allowed_actions.includes("reply")
    ? { thread: page, canWrite: true }
    : { thread: null, canWrite: false }
}

export function discussionModel(
  threads: readonly ThreadWithMessages[],
  canComment: boolean,
  replyingTo: Pick<Message, "thread_id"> | null
): DiscussionModel {
  return {
    messages: threads.flatMap((entry) => entry.messages).sort(byCreation),
    posting: postingOf(threads, canComment, replyingTo),
  }
}

export function messageVerb(message: Message): string {
  if (message.reply_to !== null) {
    return `replied to ${message.reply_to.actor.label}`
  }
  if (message.id !== message.thread_id) return "replied"
  if (!message.mentions.some((mention) => mention.kind !== "user")) {
    return "commented"
  }
  return message.run ? "asked the agent" : "mentioned the agent"
}

function byTime(a: DiscussionEntry, b: DiscussionEntry): number {
  const gap = Date.parse(a.at) - Date.parse(b.at)
  if (gap !== 0) return gap
  if (a.kind !== b.kind) return a.kind === "change" ? -1 : 1
  return a.key.localeCompare(b.key)
}

export function quotedText(excerpt: string): string {
  return mentionSegments(excerpt)
    .map((segment) =>
      segment.kind === "text" ? segment.text : `@${segment.name}`
    )
    .join("")
}

export function discussionEntries(
  messages: readonly Message[],
  changes: readonly DiscussionChange[]
): DiscussionEntry[] {
  const entries: DiscussionEntry[] = [
    ...messages.map((message): DiscussionEntry => ({
      kind: "message",
      key: `message:${message.id}`,
      at: message.created_at,
      message,
      verb: messageVerb(message),
    })),
    ...changes.map((change): DiscussionEntry => ({
      kind: "change",
      key: `change:${change.key}`,
      at: change.at,
      change,
    })),
  ]
  return entries.sort(byTime)
}

export function messageCount(threads: readonly ThreadWithMessages[]): number {
  return threads.reduce((total, entry) => total + entry.messages.length, 0)
}
