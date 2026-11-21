import { useMemo } from "react"
import { Popover as PopoverPrimitive } from "@base-ui/react/popover"

import { AuthorMark } from "@/components/author-mark"
import { clientNameFrom } from "@/activity/client-name"
import { Composer } from "@/conversation/composer"
import {
  deliveryGate,
  threadPlace,
  type GateInput,
} from "@/conversation/delivery"
import { ThreadView } from "@/conversation/thread-view"
import { useComposer } from "@/conversation/use-composer"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { markOf } from "@/lib/author"
import { plainText } from "@/search/plain-text"
import { cn } from "@/lib/utils"
import type { Size } from "@/panels/anchors"
import {
  useAnchorsModule,
  usePublished,
  type AnchorLayoutStore,
} from "@/panels/anchor-layout"

import { placePin, type PinPlacement } from "./canvas-target"
import { threadParticipants } from "./mentions"
import { plainTitle } from "./plain-title"
import {
  useThreads,
  type Anchor,
  type ThreadWithMessages,
} from "./thread-queries"

const pinSize = 22

const goneText =
  "The data this comment pointed at is no longer on the panel, so it sits where it was placed."

export type PanelPin = {
  entry: ThreadWithMessages
  anchor: Anchor
  placement: PinPlacement
}

export function panelPins(
  threads: readonly ThreadWithMessages[],
  panelId: string,
  place: (anchor: Anchor) => PinPlacement
): PanelPin[] {
  return threads.flatMap((entry) => {
    const { anchor, panel_id: onPanel } = entry.thread
    if (anchor === null || onPanel !== panelId) return []
    if (entry.messages.length === 0) return []
    return [{ entry, anchor, placement: place(anchor) }]
  })
}

export function pinLabel(pin: PanelPin): string {
  const root = pin.entry.messages[0]
  const excerpt = plainText(plainTitle(root.body)).slice(0, 80)
  const parts = [`Comment by ${root.actor.label}: ${excerpt}`]
  if (pin.entry.thread.status === "resolved") parts.push("resolved")
  if (pin.placement.gone) parts.push("its data is no longer on the panel")
  return parts.join(", ")
}

function PinThread({
  pin,
  dashboardId,
  panelTitle,
  gateInput,
}: {
  pin: PanelPin
  dashboardId: string
  panelTitle: string
  gateInput: GateInput
}) {
  const { thread, messages } = pin.entry
  const target = `dashboard:${dashboardId}`
  const people = threadParticipants(messages.map((message) => message.actor))
  const canReply = thread.allowed_actions.includes("reply")
  const model = useComposer({
    subject: { kind: "dashboard", id: dashboardId },
    place: threadPlace(thread.id, thread),
    gate: gateInput,
    people,
    onSent: () => undefined,
  })
  const gate = deliveryGate(gateInput, { to: "agent", visibleTo: "everyone" })
  return (
    <div className="flex max-h-[min(32rem,70vh)] flex-col gap-2 overflow-y-auto">
      {pin.placement.gone ? (
        <p className="text-xs text-muted-foreground">{goneText}</p>
      ) : null}
      <ThreadView
        entry={pin.entry}
        target={target}
        panelTitle={panelTitle}
        gate={gate}
        clientName={clientNameFrom(null)}
        onReplyHere={() => model.field.current?.focus()}
      />
      {canReply ? (
        <Composer
          model={model}
          layout="docked"
          label="Reply"
          formLabel="Reply in this thread"
          placeholder="Reply in this thread"
        />
      ) : null}
    </div>
  )
}

function Pin({
  pin,
  dashboardId,
  panelTitle,
  gateInput,
}: {
  pin: PanelPin
  dashboardId: string
  panelTitle: string
  gateInput: GateInput
}) {
  const id = pin.entry.thread.id
  const open = useCanvasTool((state) => state.openThreadId === id)
  const arriving = useCanvasTool((state) => state.postedThreadId === id)
  const root = pin.entry.messages[0]
  const resolved = pin.entry.thread.status === "resolved"
  const label = pinLabel(pin)
  return (
    <PopoverPrimitive.Root
      open={open}
      onOpenChange={(next) => {
        const store = useCanvasTool.getState()
        if (next) store.openThread(id)
        else if (store.openThreadId === id) store.close()
      }}
    >
      <PopoverPrimitive.Trigger
        aria-label={label}
        title={pin.placement.gone ? goneText : undefined}
        data-pin={id}
        data-resolved={resolved ? "" : undefined}
        data-gone={pin.placement.gone ? "" : undefined}
        className={cn(
          "pointer-events-auto absolute grid place-items-center rounded-[50%_50%_50%_2px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
          arriving &&
            "animate-in ease-out animation-duration-160 fade-in-0 motion-safe:zoom-in-90",
          pin.placement.gone && "opacity-60"
        )}
        style={{
          left: pin.placement.at.x,
          top: pin.placement.at.y - pinSize,
          width: pinSize,
          height: pinSize,
        }}
        onAnimationEnd={() => useCanvasTool.getState().settled(id)}
      >
        <AuthorMark
          mark={markOf(root.actor)}
          className={cn(
            "size-full rounded-[50%_50%_50%_2px] border border-card",
            resolved && "bg-muted text-muted-foreground"
          )}
        />
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Positioner
          side="right"
          align="start"
          sideOffset={6}
          collisionPadding={8}
          className="isolate z-50"
        >
          <PopoverPrimitive.Popup
            aria-label={`Thread on ${panelTitle}`}
            className="w-80 rounded-card border border-border bg-popover p-2 text-popover-foreground outline-hidden"
          >
            <PinThread
              pin={pin}
              dashboardId={dashboardId}
              panelTitle={panelTitle}
              gateInput={gateInput}
            />
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}

type Props = {
  dashboardId: string
  panelId: string
  panelTitle: string
  store: AnchorLayoutStore
  body: Size
  dataShown: boolean
  gateInput: GateInput
}

export function CanvasPins({
  dashboardId,
  panelId,
  panelTitle,
  store,
  body,
  dataShown,
  gateInput,
}: Props) {
  const { threads } = useThreads(`dashboard:${dashboardId}`)
  const published = usePublished(store)
  const module = useAnchorsModule(published?.type ?? null)
  const pins = useMemo(
    () =>
      panelPins(threads, panelId, (anchor) =>
        placePin(anchor, body, { module, published }, dataShown)
      ),
    [threads, panelId, body, module, published, dataShown]
  )
  if (pins.length === 0) return null
  return (
    <div className="pointer-events-none absolute inset-0 z-20 print:hidden">
      {pins.map((pin) => (
        <Pin
          key={pin.entry.thread.id}
          pin={pin}
          dashboardId={dashboardId}
          panelTitle={panelTitle}
          gateInput={gateInput}
        />
      ))}
    </div>
  )
}
