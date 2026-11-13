import type { ChatStateFilter } from "./chat-state"

export type ChatsSearch = {
  q?: string
  state?: ChatStateFilter
  scope?: "all"
}

const maxQueryLength = 200

export function parseChatsSearch(search: Record<string, unknown>): ChatsSearch {
  const q = typeof search.q === "string" ? search.q.trim() : ""
  return {
    q: q.length > 0 && q.length <= maxQueryLength ? q : undefined,
    state:
      search.state === "running" ||
      search.state === "stopped" ||
      search.state === "done"
        ? search.state
        : undefined,
    scope: search.scope === "all" ? "all" : undefined,
  }
}
