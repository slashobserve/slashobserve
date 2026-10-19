import type { Comment } from "./queries"

export type Thread = { root: Comment; replies: Comment[] }

function byCreation(a: Comment, b: Comment): number {
  if (a.created_at !== b.created_at) return a.created_at < b.created_at ? -1 : 1
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

export function groupThreads(comments: readonly Comment[]): Thread[] {
  const threads = new Map<string, Thread>()
  for (const comment of comments) {
    if (comment.parent_id === null) {
      threads.set(comment.id, { root: comment, replies: [] })
    }
  }
  for (const comment of comments) {
    if (comment.parent_id !== null) {
      threads.get(comment.parent_id)?.replies.push(comment)
    }
  }
  const grouped = [...threads.values()]
  for (const thread of grouped) thread.replies.sort(byCreation)
  return grouped.sort((a, b) => byCreation(a.root, b.root))
}

export function openThreadCount(comments: readonly Comment[]): number {
  return comments.filter(
    (comment) => comment.parent_id === null && comment.status === "open"
  ).length
}

export function openThreadsByPanel(
  comments: readonly Comment[]
): Map<string, number> {
  const counts = new Map<string, number>()
  for (const comment of comments) {
    if (
      comment.parent_id !== null ||
      comment.status !== "open" ||
      comment.panel_id === null
    ) {
      continue
    }
    counts.set(comment.panel_id, (counts.get(comment.panel_id) ?? 0) + 1)
  }
  return counts
}
