import type { RefObject } from "react"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import type { Point } from "@/panels/anchors"

import { AskPopover } from "./ask-popover"
import { CanvasComposer } from "./canvas-composer"
import { useCanvasHost } from "./canvas-host"

type Spot = { ownerId: string; surface: string | null }

export type SpotOwner = { ownerId: string; surfaceId: string; copy: boolean }

export function ownsSpot<T extends Spot>(
  spot: T | null,
  owner: SpotOwner
): spot is T {
  if (spot === null || spot.ownerId !== owner.ownerId) return false
  return spot.surface === null ? !owner.copy : spot.surface === owner.surfaceId
}

export function spotKeyOf(spot: Spot & { at: Point | null }): string {
  return `${spot.ownerId}:${spot.at?.x ?? ""}:${spot.at?.y ?? ""}`
}

type Props = {
  owner: SpotOwner
  panelTitle: string
  surfaceRef: RefObject<HTMLElement | null>
}

export function CanvasSpots({ owner, panelTitle, surfaceRef }: Props) {
  const host = useCanvasHost()
  const draft = useCanvasTool((state) =>
    ownsSpot(state.draft, owner) ? state.draft : null
  )
  const openTurn = useCanvasTool((state) =>
    ownsSpot(state.openTurn, owner) ? state.openTurn : null
  )
  if (host === null || host.gate.layoutEditing) return null
  const askSpot = openTurn ?? (draft?.tool === "ask" ? draft : null)
  if (askSpot !== null) {
    return (
      <AskPopover
        key={spotKeyOf(askSpot)}
        subject={host.subject}
        panelTitle={panelTitle}
        surfaceRef={surfaceRef}
        gate={host.gate}
        spot={askSpot}
        turn={openTurn}
        openChatId={host.chat.chatId}
        onChatStarted={host.chat.select}
      />
    )
  }
  if (draft?.tool !== "comment") return null
  return (
    <CanvasComposer
      key={spotKeyOf(draft)}
      subject={host.subject}
      panelTitle={panelTitle}
      draft={draft}
      surfaceRef={surfaceRef}
      gate={host.gate}
    />
  )
}
