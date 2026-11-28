import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react"

import { useViewportWidth } from "@/activity/activity-store"
import { closeInboxIfOpen, useInboxPanelOpen } from "@/inbox/inbox-panel-store"
import { cn } from "@/lib/utils"

import { isNarrow, type WidthBounds } from "./card-width"
import { panelLeft, panelShown } from "./open-panels"
import { ResizeHandle } from "./resize-handle"

type DockStacking = boolean | "container" | "sheet"

type Props = {
  panelKey: string
  labelledBy: string
  id?: string
  width: number
  bounds: WidthBounds
  onWidthChange: (width: number) => void
  stacked: DockStacking
  hidden?: boolean | undefined
  isInbox?: boolean
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void
  ref?: Ref<HTMLElement>
  children: ReactNode
}

const frameClass =
  "relative flex min-h-0 min-w-0 flex-col bg-background ease-panel motion-safe:animate-in motion-safe:animation-duration-(--duration-panel) motion-safe:fade-in-0"

const modeClass = {
  sheet:
    "motion-safe:slide-in-from-bottom-2 fixed inset-x-0 bottom-0 z-40 h-[85svh] rounded-t-card border-t data-[peek]:h-14 data-[peek]:overflow-hidden data-[peek]:[&>:not([data-slot^=dock])]:invisible",
  stacked:
    "motion-safe:slide-in-from-right-2 h-[70svh] w-full flex-none border-t",
  docked:
    "motion-safe:slide-in-from-right-2 max-h-full max-w-full flex-[0_0_var(--dock-w)] border-l",
  container:
    "motion-safe:slide-in-from-right-2 max-h-full max-w-full flex-[0_0_min(var(--dock-w),calc(100cqw_-_560px))] border-l @max-[1000px]:max-h-none @max-[1000px]:flex-[1_1_100%] @max-[1000px]:border-t @max-[1000px]:border-l-0",
}

export function DockPanel({
  panelKey,
  labelledBy,
  id,
  width,
  bounds,
  onWidthChange,
  stacked,
  hidden,
  isInbox = false,
  onKeyDown,
  ref,
  children,
}: Props) {
  const yieldsToInbox = useInboxPanelOpen() && !isInbox
  const shown = !hidden && !isInbox
  const unmounting = useRef(false)
  useEffect(() => {
    unmounting.current = false
    return () => {
      unmounting.current = true
    }
  }, [])
  useEffect(() => {
    if (!shown) return
    if (panelShown(panelKey)) closeInboxIfOpen()
    return () => panelLeft(panelKey, unmounting.current)
  }, [shown, panelKey])
  const narrow = isNarrow(useViewportWidth())
  const [peek, setPeek] = useState(false)
  const mode =
    narrow || stacked === "sheet"
      ? "sheet"
      : stacked === "container"
        ? "container"
        : stacked
          ? "stacked"
          : "docked"
  const resizable = mode === "docked" || mode === "container"

  return (
    <aside
      ref={ref}
      id={id}
      hidden={hidden || yieldsToInbox}
      aria-labelledby={labelledBy}
      onKeyDown={onKeyDown}
      data-sheet={mode === "sheet" ? "" : undefined}
      data-peek={mode === "sheet" && peek ? "" : undefined}
      style={{ "--dock-w": `${width}px` } as CSSProperties}
      className={cn("group/dock", frameClass, modeClass[mode])}
    >
      {mode === "sheet" ? (
        <button
          type="button"
          data-slot="dock-grab"
          aria-label={peek ? "Expand panel" : "Collapse panel"}
          onClick={() => setPeek((current) => !current)}
          className="grid h-5 w-full flex-none cursor-pointer place-items-center rounded-t-card outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        >
          <span
            aria-hidden="true"
            className="h-1 w-9 rounded-sm bg-muted-foreground/45"
          />
        </button>
      ) : null}
      {resizable ? (
        <ResizeHandle
          width={width}
          bounds={bounds}
          onChange={onWidthChange}
          className={mode === "container" ? "@max-[1000px]:hidden" : undefined}
        />
      ) : null}
      {children}
    </aside>
  )
}
