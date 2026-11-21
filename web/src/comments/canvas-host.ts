import { createContext, useContext } from "react"

import type { ChatListFilters } from "@/api/keys"
import type { GateInput } from "@/conversation/delivery"
import type { SpotTarget } from "@/conversation/send-plan"
import type { Anchor } from "@/panels/anchors"

export type CanvasSubject = { kind: "dashboard" | "observation"; id: string }

type AskChat = {
  chatId: string | null
  select: (chatId: string) => void
}

export type CanvasHost = {
  subject: CanvasSubject
  gate: GateInput
  chat: AskChat
}

const CanvasHostContext = createContext<CanvasHost | null>(null)

export const CanvasHostProvider = CanvasHostContext.Provider

export function useCanvasHost(): CanvasHost | null {
  return useContext(CanvasHostContext)
}

type Placed = { panel_id?: string | null; block_id?: string | null }

export function sitsOn(
  subject: CanvasSubject,
  ownerId: string,
  row: Placed
): boolean {
  return subject.kind === "dashboard"
    ? row.panel_id === ownerId
    : row.block_id === ownerId
}

export function spotTarget(
  subject: CanvasSubject,
  ownerId: string,
  anchor: Anchor | null
): SpotTarget {
  const at = anchor === null ? {} : { anchor }
  return subject.kind === "dashboard"
    ? { panelId: ownerId, ...at }
    : { blockId: ownerId, ...at }
}

export function askedHere(subject: CanvasSubject): ChatListFilters {
  return subject.kind === "dashboard"
    ? { dashboardId: subject.id, anchored: true }
    : { observationId: subject.id, anchored: true }
}
