import type { ContextChip } from "@/chat/context-chips"
import { anchorParts } from "@/conversation/anchor"
import type { Anchor } from "@/panels/anchors"

export type SpotTurn = {
  id: string
  chat_id: string | null
  panel_id: string | null
  anchor?: Anchor | null
  requested_by: { id: string }
  created_at: string
  read_only?: boolean
}

export type PrivatePin = {
  key: string
  requestId: string
  chatId: string
  anchor: Anchor
  turns: number
}

function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical)
  if (typeof value !== "object" || value === null) return value
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, canonical((value as Record<string, unknown>)[key])])
  )
}

export function spotKey(anchor: Anchor): string {
  return JSON.stringify(canonical(anchor))
}

function byTime(a: SpotTurn, b: SpotTurn): number {
  return Date.parse(a.created_at) - Date.parse(b.created_at)
}

export function privatePins(
  turns: readonly SpotTurn[],
  panelId: string,
  viewerId: string
): PrivatePin[] {
  const pins = new Map<string, PrivatePin>()
  for (const turn of [...turns].sort(byTime)) {
    const { anchor, chat_id: chatId } = turn
    if (anchor == null || chatId === null) continue
    if (turn.panel_id !== panelId || turn.requested_by.id !== viewerId) continue
    if (turn.read_only === true) continue
    const key = `${chatId}:${spotKey(anchor)}`
    const pin = pins.get(key)
    if (pin === undefined) {
      pins.set(key, { key, requestId: turn.id, chatId, anchor, turns: 1 })
    } else {
      pin.turns += 1
    }
  }
  return [...pins.values()]
}

export function spotTurnIds(
  turns: readonly SpotTurn[],
  opened: { requestId: string; anchor: Anchor | null },
  askedHere: readonly string[]
): string[] {
  const key = opened.anchor === null ? null : spotKey(opened.anchor)
  const atSpot = [...turns]
    .sort(byTime)
    .filter(
      (turn) =>
        turn.id === opened.requestId ||
        (key !== null && turn.anchor != null && spotKey(turn.anchor) === key)
    )
    .map((turn) => turn.id)
  const ids = [...atSpot]
  for (const id of [opened.requestId, ...askedHere]) {
    if (!ids.includes(id)) ids.push(id)
  }
  return ids
}

function panelName(panelTitle: string): string {
  return panelTitle === "" ? "Untitled panel" : panelTitle
}

function capitalized(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

export function askChips(
  panelTitle: string,
  anchor: Anchor | null
): ContextChip[] {
  const panel = panelName(panelTitle)
  if (anchor === null) {
    return [{ id: "panel", label: `${panel}, whole panel` }]
  }
  const parts = anchorParts(anchor)
  return [
    { id: "panel", label: panel },
    { id: "anchor", label: capitalized(parts.subject) },
    ...(parts.shown === null ? [] : [{ id: "shown", label: parts.shown }]),
    ...(parts.window === null ? [] : [{ id: "window", label: parts.window }]),
  ]
}

export function spotLabel(panelTitle: string, anchor: Anchor | null): string {
  return askChips(panelTitle, anchor)
    .map((chip) => chip.label)
    .join(" · ")
}

export function pinName(panelTitle: string, anchor: Anchor | null): string {
  const spot = askChips(panelTitle, anchor)
    .map((chip) => chip.label)
    .join(", ")
  return `Your question on ${spot}`
}

export function askWorkingText(request: {
  status: "queued" | "running" | "succeeded" | "failed"
  intent: "ask" | "apply"
}): string {
  if (request.intent === "apply") return "Revising the dashboard"
  return request.status === "queued"
    ? "Waiting for a free agent"
    : "Looking into it"
}
