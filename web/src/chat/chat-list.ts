import type { RequestSummary } from "@/observe/queries"

import { noSubjectLabel, subjectLabel } from "./chat-rows"

export type ChatGroup = {
  key: string
  label: string
  dashboardId: string | null
  requests: RequestSummary[]
}

function groupOf(request: RequestSummary): { key: string; label: string } {
  const label = subjectLabel(request) ?? noSubjectLabel
  if (request.dashboard_id !== null) {
    return { key: `dashboard:${request.dashboard_id}`, label }
  }
  if (request.observation_id !== null) {
    return { key: `observation:${request.observation_id}`, label }
  }
  return { key: "dashboard:none", label }
}

export function groupChats(requests: readonly RequestSummary[]): ChatGroup[] {
  const pinned: RequestSummary[] = []
  const byGroup = new Map<string, ChatGroup>()
  for (const request of requests) {
    if (request.pinned_at !== null) {
      pinned.push(request)
      continue
    }
    const { key, label } = groupOf(request)
    const group = byGroup.get(key)
    if (group === undefined) {
      byGroup.set(key, {
        key,
        label,
        dashboardId: request.dashboard_id,
        requests: [request],
      })
    } else {
      group.requests.push(request)
    }
  }
  const groups = [...byGroup.values()]
  if (pinned.length === 0) return groups
  return [
    { key: "pinned", label: "Pinned", dashboardId: null, requests: pinned },
    ...groups,
  ]
}
