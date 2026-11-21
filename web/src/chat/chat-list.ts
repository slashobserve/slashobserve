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
