import { useMemo } from "react"
import { useRouteContext } from "@tanstack/react-router"
import { AtSignIcon } from "lucide-react"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import { cn } from "@/lib/utils"
import { useMyRequests } from "@/observe/queries"
import type { Size } from "@/panels/anchors"
import {
  useAnchorsModule,
  usePublished,
  type AnchorLayoutStore,
} from "@/panels/anchor-layout"

import { pinName, privatePins, type PrivatePin } from "./ask-spot"
import { placePin, type PinPlacement } from "./canvas-target"

const pinSize = 22
const listedTurns = 100

type Props = {
  dashboardId: string
  panelId: string
  panelTitle: string
  surfaceId: string
  store: AnchorLayoutStore
  body: Size
  dataShown: boolean
}

function AskPin({
  pin,
  placement,
  panelId,
  panelTitle,
  surfaceId,
}: {
  pin: PrivatePin
  placement: PinPlacement
  panelId: string
  panelTitle: string
  surfaceId: string
}) {
  const open = useCanvasTool(
    (state) => state.openTurn?.requestId === pin.requestId
  )
  const label = pinName(panelTitle, pin.anchor)
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-expanded={open}
      data-ask-pin={pin.requestId}
      data-gone={placement.gone ? "" : undefined}
      className={cn(
        "pointer-events-auto absolute grid place-items-center rounded-[50%_50%_50%_2px] border border-card bg-primary text-primary-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring",
        open && "outline-2 outline-offset-1 outline-primary",
        placement.gone && "opacity-60"
      )}
      style={{
        left: placement.at.x,
        top: placement.at.y - pinSize,
        width: pinSize,
        height: pinSize,
      }}
      onClick={() =>
        useCanvasTool.getState().showTurn({
          requestId: pin.requestId,
          chatId: pin.chatId,
          panelId,
          anchor: pin.anchor,
          at: placement.at,
          surface: surfaceId,
        })
      }
    >
      <AtSignIcon aria-hidden="true" className="size-3" />
    </button>
  )
}

export function AskPins({
  dashboardId,
  panelId,
  panelTitle,
  surfaceId,
  store,
  body,
  dataShown,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const { requests } = useMyRequests({ dashboardId }, listedTurns)
  const published = usePublished(store)
  const module = useAnchorsModule(published?.type ?? null)
  const pins = useMemo(
    () =>
      privatePins(requests, panelId, principal.id).map((pin) => ({
        pin,
        placement: placePin(pin.anchor, body, { module, published }, dataShown),
      })),
    [requests, panelId, principal.id, body, module, published, dataShown]
  )
  if (pins.length === 0) return null
  return (
    <div className="pointer-events-none absolute inset-0 z-20 print:hidden">
      {pins.map(({ pin, placement }) => (
        <AskPin
          key={pin.key}
          pin={pin}
          placement={placement}
          panelId={panelId}
          panelTitle={panelTitle}
          surfaceId={surfaceId}
        />
      ))}
    </div>
  )
}
