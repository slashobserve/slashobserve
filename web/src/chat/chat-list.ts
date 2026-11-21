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

function isChatTurn(request: RequestSummary): boolean {
  return request.source === "observe" || request.source === "prompt"
}

function find(parent: Map<string, string>, node: string): string {
  const up = parent.get(node)
  if (up === undefined || up === node) return node
  const root = find(parent, up)
  parent.set(node, root)
  return root
}

function chatNode(request: RequestSummary): string {
  return `chat:${request.chat_id ?? request.id}`
}

function conversationKeys(
  requests: readonly RequestSummary[]
): Map<string, string> {
  const parent = new Map<string, string>()
  const nodeOf = (request: RequestSummary) =>
    request.dashboard_id === null
      ? chatNode(request)
      : `dashboard:${request.dashboard_id}`
  const turns = requests.filter(isChatTurn)
  for (const request of turns) {
    if (request.dashboard_id === null) continue
    const chat = find(parent, chatNode(request))
    const dashboard = find(parent, nodeOf(request))
    if (chat !== dashboard) parent.set(chat, dashboard)
  }
  return new Map(
    turns.map((request) => [request.id, find(parent, nodeOf(request))])
  )
}

export function chatRoot(turns: readonly RequestSummary[]): RequestSummary {
  return (
    turns.findLast((turn) => turn.source === "observe") ??
    turns[turns.length - 1]
  )
}

export function chatTurns(
  requests: readonly RequestSummary[],
  row: RequestSummary
): RequestSummary[] {
  const keys = conversationKeys(requests)
  const key = keys.get(row.id)
  if (key === undefined) return [row]
  return requests.filter((request) => keys.get(request.id) === key)
}

export function collapseChats(
  requests: readonly RequestSummary[]
): RequestSummary[] {
  const keys = conversationKeys(requests)
  const shown = new Set<string>()
  return requests.flatMap((request) => {
    const key = keys.get(request.id)
    if (key === undefined) return [request]
    if (shown.has(key)) return []
    shown.add(key)
    const root = chatRoot(chatTurns(requests, request))
    return root.id === request.id
      ? [request]
      : [{ ...request, title: root.title }]
  })
}

function newestFirst(a: RequestSummary, b: RequestSummary): number {
  return Date.parse(b.created_at) - Date.parse(a.created_at)
}

export function newestChats(
  requests: readonly RequestSummary[],
  limit: number
): RequestSummary[] {
  return collapseChats(requests.toSorted(newestFirst)).slice(0, limit)
}
