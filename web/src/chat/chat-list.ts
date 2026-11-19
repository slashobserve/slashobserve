import type { RequestSummary } from "@/observe/queries"

export type ChatGroup = {
  key: string
  label: string
  dashboardId: string | null
  requests: RequestSummary[]
}

function dashboardLabel(request: RequestSummary): string {
  if (request.dashboard_id === null) return "No dashboard"
  return request.dashboard_title ?? "A dashboard you cannot see"
}

export function groupChats(requests: readonly RequestSummary[]): ChatGroup[] {
  const pinned: RequestSummary[] = []
  const byDashboard = new Map<string, ChatGroup>()
  for (const request of requests) {
    if (request.pinned_at !== null) {
      pinned.push(request)
      continue
    }
    const key = `dashboard:${request.dashboard_id ?? "none"}`
    const group = byDashboard.get(key)
    if (group === undefined) {
      byDashboard.set(key, {
        key,
        label: dashboardLabel(request),
        dashboardId: request.dashboard_id,
        requests: [request],
      })
    } else {
      group.requests.push(request)
    }
  }
  const groups = [...byDashboard.values()]
  if (pinned.length === 0) return groups
  return [
    { key: "pinned", label: "Pinned", dashboardId: null, requests: pinned },
    ...groups,
  ]
}

function conversationKey(request: RequestSummary): string | null {
  if (request.source !== "observe" && request.source !== "prompt") return null
  return request.dashboard_id ?? request.chat_id
}

export function collapseChats(
  requests: readonly RequestSummary[]
): RequestSummary[] {
  const roots = new Map<string, RequestSummary>()
  for (const request of requests) {
    const key = conversationKey(request)
    if (key === null) continue
    const root = roots.get(key)
    if (root === undefined || root.source !== "observe") roots.set(key, request)
    else if (request.source === "observe") roots.set(key, request)
  }
  const shown = new Set<string>()
  return requests.flatMap((request) => {
    const key = conversationKey(request)
    if (key === null) return [request]
    if (shown.has(key)) return []
    shown.add(key)
    const root = roots.get(key)
    return root === undefined || root.id === request.id
      ? [request]
      : [{ ...request, title: root.title }]
  })
}
