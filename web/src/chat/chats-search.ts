export type ChatsSearch = {
  q?: string | undefined
  dashboard?: string | undefined
  scope?: "all" | undefined
}

const maxQueryLength = 200
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function parseChatsSearch(search: Record<string, unknown>): ChatsSearch {
  const q = typeof search.q === "string" ? search.q.trim() : ""
  return {
    q: q.length > 0 && q.length <= maxQueryLength ? q : undefined,
    dashboard:
      typeof search.dashboard === "string" && uuidPattern.test(search.dashboard)
        ? search.dashboard
        : undefined,
    scope: search.scope === "all" ? "all" : undefined,
  }
}
