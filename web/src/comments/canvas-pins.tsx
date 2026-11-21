import { useMemo, useState, type ReactNode } from "react"
import { ChevronLeftIcon } from "lucide-react"
import { Popover as PopoverPrimitive } from "@base-ui/react/popover"

import { AuthorMark } from "@/components/author-mark"
import { Button } from "@/components/ui/button"
import { clientNameFrom } from "@/activity/client-name"
import { Composer } from "@/conversation/composer"
import {
  deliveryGate,
  threadPlace,
  type GateInput,
} from "@/conversation/delivery"
import { ThreadView } from "@/conversation/thread-view"
import { threadTargetOf } from "@/conversation/send-plan"
import { useComposer } from "@/conversation/use-composer"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { markOf } from "@/lib/author"
import { plainText } from "@/search/plain-text"
import { cn } from "@/lib/utils"
import type { Point, Size } from "@/panels/anchors"

import { sitsOn, type CanvasSubject } from "./canvas-host"
import {
  placePin,
  revealAnchor,
  type CanvasSurface,
  type PinPlacement,
  type PinView,
} from "./canvas-target"
import { threadParticipants } from "./mentions"
import { plainTitle } from "./plain-title"
import {
  useThreads,
  type Anchor,
  type ThreadWithMessages,
} from "./thread-queries"

const pinSize = 22

const markerShape = "rounded-[50%_50%_50%_2px]"

const goneText =
  "The data this comment pointed at is no longer on the panel, so it sits where it was placed."

const detachedText =
  "The text this comment pointed at is no longer in this block, so it is set apart in the corner."

function goneTextOf(anchor: Anchor): string {
  return anchor.kind === "text" ? detachedText : goneText
}

export type PanelPin = {
  entry: ThreadWithMessages
  anchor: Anchor
  placement: PinPlacement
}

export function panelPins(
  threads: readonly ThreadWithMessages[],
  onSurface: (thread: ThreadWithMessages["thread"]) => boolean,
  place: (anchor: Anchor) => PinPlacement | null
): PanelPin[] {
  return threads.flatMap((entry) => {
    const { anchor } = entry.thread
    if (anchor === null || !onSurface(entry.thread)) return []
    if (entry.messages.length === 0) return []
    const placement = place(anchor)
    return placement === null ? [] : [{ entry, anchor, placement }]
  })
}

export type PinStack<T = PanelPin> = { at: Point; pins: T[] }

export function stackPins<T extends { placement: PinPlacement }>(
  pins: readonly T[],
  reach = pinSize
): PinStack<T>[] {
  const stacks: PinStack<T>[] = []
  for (const pin of pins) {
    const { x, y } = pin.placement.at
    const near = stacks.find(({ at }) => Math.hypot(at.x - x, at.y - y) < reach)
    if (near === undefined) stacks.push({ at: pin.placement.at, pins: [pin] })
    else near.pins.push(pin)
  }
  return stacks
}

export function stackLabel(stack: PinStack): string {
  const resolved = stack.pins.filter(
    ({ entry }) => entry.thread.status === "resolved"
  ).length
  const count = `${stack.pins.length} comments here`
  return resolved === 0 ? count : `${count}, ${resolved} resolved`
}

export function pinLabel(pin: PanelPin): string {
  const root = pin.entry.messages[0]
  const excerpt = plainText(plainTitle(root.body)).slice(0, 80)
  const parts = [`Comment by ${root.actor.label}: ${excerpt}`]
  if (pin.entry.thread.status === "resolved") parts.push("resolved")
  if (pin.placement.gone) {
    parts.push(
      pin.anchor.kind === "text"
        ? "its text is no longer in the block"
        : "its data is no longer on the panel"
    )
  }
  return parts.join(", ")
}

function PinThread({
  pin,
  subject,
  panelTitle,
  gateInput,
}: {
  pin: PanelPin
  subject: CanvasSubject
  panelTitle: string
  gateInput: GateInput
}) {
  const { thread, messages } = pin.entry
  const target = threadTargetOf(subject)
  const people = threadParticipants(messages.map((message) => message.actor))
  const canReply = thread.allowed_actions.includes("reply")
  const model = useComposer({
    subject,
    place: threadPlace(thread.id, thread),
    gate: gateInput,
    people,
    onSent: () => undefined,
  })
  const gate = deliveryGate(gateInput, { to: "agent", visibleTo: "everyone" })
  return (
    <div className="flex max-h-[min(32rem,70vh)] flex-col gap-2 overflow-y-auto">
      {pin.placement.gone ? (
        <p className="text-xs text-muted-foreground">
          {goneTextOf(pin.anchor)}
        </p>
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

type PinProps = {
  subject: CanvasSubject
  panelTitle: string
  gateInput: GateInput
  onOpen: (anchor: Anchor) => void
}

function Pin({
  pin,
  subject,
  panelTitle,
  gateInput,
  onOpen,
}: PinProps & { pin: PanelPin }) {
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
        if (next) {
          store.openThread(id)
          onOpen(pin.anchor)
        } else if (store.openThreadId === id) store.close()
      }}
    >
      <PopoverPrimitive.Trigger
        aria-label={label}
        title={pin.placement.gone ? goneTextOf(pin.anchor) : undefined}
        data-pin={id}
        data-resolved={resolved ? "" : undefined}
        data-gone={pin.placement.gone ? "" : undefined}
        className={markerClass(arriving, pin.placement.gone)}
        style={markerStyle(pin.placement.at)}
        onAnimationEnd={() => useCanvasTool.getState().settled(id)}
      >
        <AuthorMark
          mark={markOf(root.actor)}
          className={cn(
            "size-full border border-card",
            markerShape,
            resolved && "bg-muted text-muted-foreground"
          )}
        />
      </PopoverPrimitive.Trigger>
      <PinPopup label={`Thread on ${panelTitle}`}>
        <PinThread
          pin={pin}
          subject={subject}
          panelTitle={panelTitle}
          gateInput={gateInput}
        />
      </PinPopup>
    </PopoverPrimitive.Root>
  )
}

export function markerClass(arriving: boolean, gone: boolean): string {
  return cn(
    "pointer-events-auto absolute grid place-items-center outline-none focus-visible:ring-2 focus-visible:ring-ring",
    markerShape,
    arriving &&
      "animate-in ease-out animation-duration-160 fade-in-0 motion-safe:zoom-in-90",
    gone && "opacity-60"
  )
}

export function markerStyle(at: Point) {
  return {
    left: at.x,
    top: at.y - pinSize,
    width: pinSize,
    height: pinSize,
  }
}

export function PinPopup({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Positioner
        side="right"
        align="start"
        sideOffset={6}
        collisionPadding={8}
        className="isolate z-50"
      >
        <PopoverPrimitive.Popup
          aria-label={label}
          className="w-80 rounded-card border border-border bg-popover p-2 text-popover-foreground outline-hidden"
        >
          {children}
        </PopoverPrimitive.Popup>
      </PopoverPrimitive.Positioner>
    </PopoverPrimitive.Portal>
  )
}

function StackedPins({
  stack,
  subject,
  panelTitle,
  gateInput,
  onOpen,
}: PinProps & { stack: PinStack }) {
  const ids = stack.pins.map(({ entry }) => entry.thread.id)
  const openId = useCanvasTool((state) =>
    state.openThreadId !== null && ids.includes(state.openThreadId)
      ? state.openThreadId
      : null
  )
  const arriving = useCanvasTool(
    (state) =>
      state.postedThreadId !== null && ids.includes(state.postedThreadId)
  )
  const [listing, setListing] = useState(false)
  const shown = stack.pins.find(({ entry }) => entry.thread.id === openId)
  const allResolved = stack.pins.every(
    ({ entry }) => entry.thread.status === "resolved"
  )
  const allGone = stack.pins.every(({ placement }) => placement.gone)
  return (
    <PopoverPrimitive.Root
      open={listing || shown !== undefined}
      onOpenChange={(next) => {
        setListing(next)
        if (!next && openId !== null) useCanvasTool.getState().close()
      }}
    >
      <PopoverPrimitive.Trigger
        aria-label={stackLabel(stack)}
        data-pin-stack={stack.pins.length}
        className={cn(
          markerClass(arriving, allGone),
          "border text-xs font-medium tabular-nums",
          allResolved
            ? "border-card bg-muted text-muted-foreground"
            : "border-border bg-card text-foreground"
        )}
        style={markerStyle(stack.at)}
        onAnimationEnd={() => {
          const posted = useCanvasTool.getState().postedThreadId
          if (posted !== null) useCanvasTool.getState().settled(posted)
        }}
      >
        {stack.pins.length}
      </PopoverPrimitive.Trigger>
      <PinPopup
        label={
          shown === undefined
            ? `Comments here on ${panelTitle}`
            : `Thread on ${panelTitle}`
        }
      >
        {shown === undefined ? (
          <ul className="flex flex-col gap-0.5">
            {stack.pins.map((pin) => (
              <li key={pin.entry.thread.id}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-control px-2 py-1.5 text-left text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                  onClick={() => {
                    useCanvasTool.getState().openThread(pin.entry.thread.id)
                    onOpen(pin.anchor)
                  }}
                >
                  <AuthorMark
                    mark={markOf(pin.entry.messages[0].actor)}
                    className="size-5 shrink-0"
                  />
                  <span className="min-w-0 flex-1 truncate">
                    {pinLabel(pin)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <div className="flex flex-col gap-2">
            <Button
              variant="ghost"
              size="xs"
              className="self-start"
              onClick={() => {
                setListing(true)
                useCanvasTool.getState().close()
              }}
            >
              <ChevronLeftIcon aria-hidden="true" />
              All {stack.pins.length} comments here
            </Button>
            <PinThread
              pin={shown}
              subject={subject}
              panelTitle={panelTitle}
              gateInput={gateInput}
            />
          </div>
        )}
      </PinPopup>
    </PopoverPrimitive.Root>
  )
}

type Props = {
  subject: CanvasSubject
  ownerId: string
  panelTitle: string
  surface: CanvasSurface
  body: Size
  view: PinView
  gateInput: GateInput
}

export function CanvasPins({
  subject,
  ownerId,
  panelTitle,
  surface,
  body,
  view,
  gateInput,
}: Props) {
  const { threads } = useThreads(threadTargetOf(subject))
  const stacks = useMemo(
    () =>
      stackPins(
        panelPins(
          threads,
          (thread) => sitsOn(subject, ownerId, thread),
          (anchor) => placePin(anchor, body, surface, view)
        )
      ),
    [threads, subject, ownerId, body, surface, view]
  )
  const onOpen = (anchor: Anchor) => {
    revealAnchor(surface, anchor)
  }
  if (stacks.length === 0) return null
  return (
    <div className="pointer-events-none absolute inset-0 z-20 print:hidden">
      {stacks.map((stack) =>
        stack.pins.length === 1 ? (
          <Pin
            key={stack.pins[0].entry.thread.id}
            pin={stack.pins[0]}
            subject={subject}
            panelTitle={panelTitle}
            gateInput={gateInput}
            onOpen={onOpen}
          />
        ) : (
          <StackedPins
            key={stack.pins[0].entry.thread.id}
            stack={stack}
            subject={subject}
            panelTitle={panelTitle}
            gateInput={gateInput}
            onOpen={onOpen}
          />
        )
      )}
    </div>
  )
}
