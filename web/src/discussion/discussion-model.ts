import type { Message, ThreadWithMessages } from "@/comments/thread-queries"

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
