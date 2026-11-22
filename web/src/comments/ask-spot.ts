import type { ContextChip } from "@/chat/context-chips"
import { chipParts, describeAnchor } from "@/conversation/anchor"
import type { ObserveRequest, RequestSummary } from "@/observe/queries"
import type { Anchor } from "@/panels/anchors"

export type PinTurn = Pick<
  RequestSummary,
  | "id"
  | "prompt"
  | "chat_id"
  | "panel_id"
  | "block_id"
  | "anchor"
  | "requested_by"
  | "created_at"
  | "observation_id"
  | "trigger"
>

type ChatTurn = Pick<
  ObserveRequest,
  "id" | "panel_id" | "anchor" | "retried_by" | "created_at"
>

type OpenedSpot = { requestId: string; panelId: string; anchor: Anchor | null }

function byTime(a: { created_at: string }, b: { created_at: string }): number {
  return Date.parse(a.created_at) - Date.parse(b.created_at)
}

export type PrivatePin = {
  key: string
  requestId: string
  chatId: string
  anchor: Anchor
  prompt: string
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

export function privatePins(
  turns: readonly PinTurn[],
  onSurface: (turn: PinTurn) => boolean,
  viewerId: string
): PrivatePin[] {
  const pins = new Map<string, PrivatePin>()
  for (const turn of [...turns].sort(byTime)) {
    const { anchor, chat_id: chatId } = turn
    if (anchor === null || chatId === null) continue
    if (!onSurface(turn) || turn.requested_by.id !== viewerId) continue
    const key = `${chatId}:${spotKey(anchor)}`
    const pin = pins.get(key)
    if (pin === undefined) {
      pins.set(key, {
        key,
        requestId: turn.id,
        chatId,
        anchor,
        prompt: turn.prompt,
        turns: 1,
      })
    } else {
      pin.turns += 1
    }
  }
  return [...pins.values()]
}

export function spotTurnIds(
  turns: readonly ChatTurn[],
  opened: OpenedSpot,
  askedHere: readonly string[]
): string[] {
  const key = opened.anchor === null ? null : spotKey(opened.anchor)
  const atSpot: string[] = []
  const retries = new Set<string>()
  for (const turn of [...turns].sort(byTime)) {
    const here =
      turn.id === opened.requestId ||
      retries.has(turn.id) ||
      (key !== null &&
        turn.panel_id === opened.panelId &&
        turn.anchor !== null &&
        spotKey(turn.anchor) === key)
    if (!here) continue
    atSpot.push(turn.id)
    if (turn.retried_by !== null) retries.add(turn.retried_by)
  }
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
  const parts = chipParts(anchor)
  return [
    { id: "panel", label: panel },
    {
      id: "anchor",
      label: capitalized(parts.subject),
      title: capitalized(describeAnchor(anchor)),
    },
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
