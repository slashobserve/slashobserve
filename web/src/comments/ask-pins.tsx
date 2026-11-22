import { useMemo, useState } from "react"
import { useRouteContext } from "@tanstack/react-router"
import { Popover as PopoverPrimitive } from "@base-ui/react/popover"
import { AtSignIcon } from "lucide-react"

import { useCanvasTool } from "@/dashboards/canvas-tools"
import { cn } from "@/lib/utils"
import { useMyRequests } from "@/observe/queries"
import type { Point, Size } from "@/panels/anchors"
import {
  useAnchorsModule,
  usePublished,
  type AnchorLayoutStore,
} from "@/panels/anchor-layout"

import { pinName, privatePins, type PrivatePin } from "./ask-spot"
import {
  placePin,
  revealAnchor,
  type CanvasSurface,
  type PinPlacement,
  type PinView,
} from "./canvas-target"
import {
  markerClass,
  markerStyle,
  PinPopup,
  stackPins,
  type PinStack,
} from "./canvas-pins"

const listedTurns = 100

const askMarkerClass =
  "border border-card bg-primary text-primary-foreground data-[open]:outline-2 data-[open]:outline-offset-1 data-[open]:outline-primary"

type PlacedPin = { pin: PrivatePin; placement: PinPlacement }

type Spot = {
  panelId: string
  panelTitle: string
  surfaceId: string
  surface: CanvasSurface
}

function askStackLabel(count: number): string {
  return `${count} of your questions here`
}

function openPin(spot: Spot, pin: PrivatePin, at: Point) {
  revealAnchor(spot.surface, pin.anchor)
  useCanvasTool.getState().showTurn({
    requestId: pin.requestId,
    chatId: pin.chatId,
    panelId: spot.panelId,
    anchor: pin.anchor,
    at,
    surface: spot.surfaceId,
  })
}

function AskPin({ placed, spot }: { placed: PlacedPin; spot: Spot }) {
  const { pin, placement } = placed
  const open = useCanvasTool(
    (state) => state.openTurn?.requestId === pin.requestId
  )
  const label = pinName(spot.panelTitle, pin.anchor)
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-expanded={open}
      data-ask-pin={pin.requestId}
      data-gone={placement.gone ? "" : undefined}
      data-open={open ? "" : undefined}
      className={cn(markerClass(false, placement.gone), askMarkerClass)}
      style={markerStyle(placement.at)}
      onClick={() => openPin(spot, pin, placement.at)}
    >
      <AtSignIcon aria-hidden="true" className="size-3" />
    </button>
  )
}

function AskPinStack({
  stack,
  spot,
}: {
  stack: PinStack<PlacedPin>
  spot: Spot
}) {
  const [listing, setListing] = useState(false)
  const ids = stack.pins.map(({ pin }) => pin.requestId)
  const open = useCanvasTool(
    (state) => state.openTurn !== null && ids.includes(state.openTurn.requestId)
  )
  const gone = stack.pins.every(({ placement }) => placement.gone)
  return (
    <PopoverPrimitive.Root open={listing} onOpenChange={setListing}>
      <PopoverPrimitive.Trigger
        aria-label={askStackLabel(stack.pins.length)}
        data-ask-pin-stack={stack.pins.length}
        data-open={open ? "" : undefined}
        className={cn(
          markerClass(false, gone),
          askMarkerClass,
          "text-xs font-medium tabular-nums"
        )}
        style={markerStyle(stack.at)}
      >
        {stack.pins.length}
      </PopoverPrimitive.Trigger>
      <PinPopup label={`Your questions here on ${spot.panelTitle}`}>
        <ul className="flex flex-col gap-0.5">
          {stack.pins.map(({ pin }) => (
            <li key={pin.key}>
              <button
                type="button"
                className="flex w-full flex-col items-start rounded-control px-2 py-1.5 text-left text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => {
                  setListing(false)
                  openPin(spot, pin, stack.at)
                }}
              >
                <span className="w-full truncate">{pin.prompt}</span>
                <span className="w-full truncate text-xs text-muted-foreground">
                  {pinName(spot.panelTitle, pin.anchor)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </PinPopup>
    </PopoverPrimitive.Root>
  )
}

type Props = {
  dashboardId: string
  panelId: string
  panelTitle: string
  surfaceId: string
  store: AnchorLayoutStore
  body: Size
  view: PinView
}

export function AskPins({
  dashboardId,
  panelId,
  panelTitle,
  surfaceId,
  store,
  body,
  view,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const { requests } = useMyRequests(
    { dashboardId, anchored: true },
    listedTurns,
    true,
    false
  )
  const published = usePublished(store)
  const module = useAnchorsModule(published?.type ?? null)
  const surface = useMemo(() => ({ module, published }), [module, published])
  const stacks = useMemo(
    () =>
      stackPins(
        privatePins(requests, panelId, principal.id).flatMap((pin) => {
          const placement = placePin(pin.anchor, body, surface, view)
          return placement === null ? [] : [{ pin, placement }]
        })
      ),
    [requests, panelId, principal.id, body, surface, view]
  )
  if (stacks.length === 0) return null
  const spot: Spot = { panelId, panelTitle, surfaceId, surface }
  return (
    <div className="pointer-events-none absolute inset-0 z-20 print:hidden">
      {stacks.map((stack) =>
        stack.pins.length === 1 ? (
          <AskPin
            key={stack.pins[0].pin.key}
            placed={stack.pins[0]}
            spot={spot}
          />
        ) : (
          <AskPinStack key={stack.pins[0].pin.key} stack={stack} spot={spot} />
        )
      )}
    </div>
  )
}
