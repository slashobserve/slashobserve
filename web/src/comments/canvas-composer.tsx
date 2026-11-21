import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react"
import { Popover as PopoverPrimitive } from "@base-ui/react/popover"
import { useSearch } from "@tanstack/react-router"

import { ContextChips, type ContextChip } from "@/chat/context-chips"
import { describeAnchor } from "@/conversation/anchor"
import { Composer } from "@/conversation/composer"
import type { GateInput } from "@/conversation/delivery"
import { useComposer } from "@/conversation/use-composer"
import {
  useCanvasTool,
  type CanvasDraft,
  type DraftTool,
} from "@/dashboards/canvas-tools"
import { validateDashboardSearch } from "@/dashboards/search"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import type { Anchor, Point, Rect } from "@/panels/anchors"
import { useAnchorsModule, usePublished } from "@/panels/anchor-layout"
import type { AnchorLayoutStore } from "@/panels/anchor-layout"

import {
  dragAnchor,
  outlineOf,
  pointerAnchor,
  titleAnchor,
  withWindow,
  type CanvasSurface,
} from "./canvas-target"

const keyDraftInset = 12

const placeholders: Record<DraftTool, string> = {
  comment: "Write a comment",
  ask: "Ask about this",
}

const formLabels: Record<DraftTool, string> = {
  comment: "Comment on the canvas",
  ask: "Ask on the canvas",
}

const clickedPopupClass =
  "transition-[opacity,scale] duration-(--duration-fast) ease-out data-ending-style:opacity-0 data-ending-style:duration-100 data-starting-style:opacity-0 motion-safe:data-ending-style:scale-96 motion-safe:data-starting-style:scale-96"

export function targetChips(
  panelTitle: string,
  anchor: Anchor | null
): ContextChip[] {
  const panel: ContextChip = {
    id: "panel",
    label: panelTitle === "" ? "Untitled panel" : panelTitle,
  }
  if (anchor === null)
    return [{ ...panel, label: `${panel.label}, whole panel` }]
  const described = describeAnchor(anchor)
  return [
    panel,
    {
      id: "anchor",
      label: described.charAt(0).toUpperCase() + described.slice(1),
    },
  ]
}

export function useCanvasGate(
  canComment: boolean,
  layoutEditing: boolean
): GateInput {
  const pinned = useSearch({
    strict: false,
    select: (search: Record<string, unknown>) =>
      validateDashboardSearch(search).version !== undefined,
  })
  return {
    canComment,
    agentAvailable: useAgentInfo().data?.available,
    pinned,
    layoutEditing,
  }
}

function spotAnchor(surface: HTMLElement | null, at: Point | null) {
  const offset = at ?? { x: keyDraftInset, y: keyDraftInset }
  return {
    contextElement: surface ?? undefined,
    getBoundingClientRect: () => {
      const box = surface?.getBoundingClientRect()
      return DOMRect.fromRect({
        x: (box?.left ?? 0) + offset.x,
        y: (box?.top ?? 0) + offset.y,
        width: 0,
        height: 0,
      })
    },
  }
}

const leavingReasons: ReadonlySet<string> = new Set([
  "outside-press",
  "focus-out",
])

function keepsTyping(reason: string, text: string): boolean {
  return leavingReasons.has(reason) && text.trim() !== ""
}

type ComposerProps = {
  dashboardId: string
  panelTitle: string
  draft: CanvasDraft
  surfaceRef: RefObject<HTMLElement | null>
  gate: GateInput
}

export function CanvasComposer({
  dashboardId,
  panelTitle,
  draft,
  surfaceRef,
  gate,
}: ComposerProps) {
  const posted = useCanvasTool((state) => state.posted)
  const text = useCanvasTool((state) => state.text)
  const setText = useCanvasTool((state) => state.setText)
  const model = useComposer({
    draft: { value: text, onChange: setText },
    subject: { kind: "dashboard", id: dashboardId },
    place: { kind: "canvas", tool: draft.tool },
    target: {
      panelId: draft.panelId,
      ...(draft.anchor === null ? {} : { anchor: draft.anchor }),
    },
    gate,
    onSent: (sent) => {
      if (sent.via === "thread") posted(sent.sent.thread.id)
      else useCanvasTool.getState().close()
    },
  })
  const at = draft.at
  const anchor = useCallback(
    () => spotAnchor(surfaceRef.current, at),
    [surfaceRef, at]
  )
  const clicked = draft.at !== null
  return (
    <PopoverPrimitive.Root
      open
      onOpenChange={(open, details) => {
        const store = useCanvasTool.getState()
        if (open || store.draft !== draft) return
        if (keepsTyping(details.reason, store.text)) return
        store.close()
      }}
    >
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Positioner
          anchor={anchor}
          side="bottom"
          align="start"
          sideOffset={6}
          collisionPadding={8}
          className="isolate z-50"
        >
          <PopoverPrimitive.Popup
            aria-label={formLabels[draft.tool]}
            initialFocus={model.field}
            data-opened-by={clicked ? "pointer" : "key"}
            className={cn(
              "w-80 origin-(--transform-origin) rounded-card border border-border bg-popover p-2 text-popover-foreground outline-hidden",
              clicked && clickedPopupClass
            )}
          >
            <Composer
              model={model}
              layout="docked"
              label={placeholders[draft.tool]}
              formLabel={formLabels[draft.tool]}
              placeholder={placeholders[draft.tool]}
              chips={
                <ContextChips chips={targetChips(panelTitle, draft.anchor)} />
              }
            />
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}

const panelBody = "[data-canvas-surface]"
const pinMarker = "[data-pin], [data-pin-stack]"
const headerControls =
  'a, input, select, textarea, [role="menuitem"], [role="toolbar"], button:not([data-panel-title])'

type Hover = { outline: Rect | null; offset: Point } | null

type LayerProps = {
  containerRef: RefObject<HTMLElement | null>
  surfaceRef: RefObject<HTMLElement | null>
  store: AnchorLayoutStore
  surfaceId: string
  panelId: string
  tool: DraftTool
  body: { width: number; height: number }
  shownRange: { from: string; to: string }
  wholePanel?: boolean
}

function useSurface(store: AnchorLayoutStore): CanvasSurface {
  const published = usePublished(store)
  const module = useAnchorsModule(published?.type ?? null)
  return { module, published }
}

function pointIn(element: HTMLElement, event: PointerEvent): Point {
  const box = element.getBoundingClientRect()
  return { x: event.clientX - box.left, y: event.clientY - box.top }
}

function pressedByKeyboard(event: MouseEvent): boolean {
  return event.detail === 0
}

function offsetOf(container: HTMLElement, surface: HTMLElement): Point {
  const outer = container.getBoundingClientRect()
  const inner = surface.getBoundingClientRect()
  return { x: inner.left - outer.left, y: inner.top - outer.top }
}

export function CanvasLayer({
  containerRef,
  surfaceRef,
  store,
  surfaceId,
  panelId,
  tool,
  body,
  shownRange,
  wholePanel = false,
}: LayerProps) {
  const surface = useSurface(store)
  const [hover, setHover] = useState<Hover>(null)
  const current = useRef({ surface, body, shownRange, wholePanel })
  useLayoutEffect(() => {
    current.current = { surface, body, shownRange, wholePanel }
  })

  useEffect(() => {
    const container = containerRef.current
    if (container === null) return
    function inBody(event: Event): boolean {
      const target = event.target
      return target instanceof Element && target.closest(panelBody) !== null
    }
    function onPin(event: Event): boolean {
      const target = event.target
      return target instanceof Element && target.closest(pinMarker) !== null
    }
    function targetOf(event: PointerEvent): Anchor | null {
      const element = surfaceRef.current
      if (element === null) return null
      const { surface, body, shownRange, wholePanel } = current.current
      const point = pointIn(element, event)
      if (wholePanel && inBody(event)) return null
      const anchor = inBody(event)
        ? pointerAnchor(surface, point, body)
        : titleAnchor(point, body)
      return withWindow(anchor, shownRange)
    }
    let press: { at: Point; anchor: Anchor | null } | null = null
    function dragged(event: PointerEvent, element: HTMLElement) {
      if (press === null || current.current.wholePanel) return null
      const anchor = dragAnchor(
        current.current.surface,
        press.at,
        pointIn(element, event)
      )
      return anchor === null
        ? null
        : withWindow(anchor, current.current.shownRange)
    }
    function onMove(event: PointerEvent) {
      const element = surfaceRef.current
      if (element === null || container === null) return
      const target = event.target
      if (!(target instanceof Node) || !container.contains(target)) return
      if (press === null && onPin(event)) return setHover(null)
      const anchor =
        dragged(event, element) ?? (inBody(event) ? targetOf(event) : null)
      setHover({
        outline: outlineOf(current.current.surface, anchor),
        offset: offsetOf(container, element),
      })
    }
    function onLeave() {
      if (press === null) setHover(null)
    }
    function claims(event: Event): boolean {
      const target = event.target
      if (!(target instanceof Element) || container === null) return false
      if (!container.contains(target) || onPin(event)) return false
      return inBody(event) || target.closest(headerControls) === null
    }
    function onDown(event: PointerEvent) {
      if (event.button !== 0 || !claims(event)) return
      event.preventDefault()
      event.stopPropagation()
      const element = surfaceRef.current
      if (element === null) return
      press = { at: pointIn(element, event), anchor: targetOf(event) }
    }
    function onUp(event: PointerEvent) {
      const element = surfaceRef.current
      if (press === null || element === null) return
      const range = dragged(event, element)
      const end = pointIn(element, event)
      const { at, anchor } = press
      press = null
      const target = event.target
      if (!(target instanceof Node) || !container?.contains(target)) {
        setHover(null)
      }
      useCanvasTool.getState().openDraft({
        panelId,
        tool,
        anchor: range ?? anchor,
        at: range === null ? at : end,
        surface: surfaceId,
      })
    }
    function onCancel() {
      press = null
    }
    function onClick(event: MouseEvent) {
      if (pressedByKeyboard(event) || !claims(event)) return
      event.preventDefault()
      event.stopPropagation()
    }
    function onKey(event: KeyboardEvent) {
      if (event.key !== "Enter" && event.key !== " ") return
      const target = event.target
      if (!(target instanceof Element)) return
      if (target.closest("[data-panel-title]") === null) return
      event.preventDefault()
      event.stopPropagation()
      useCanvasTool.getState().openDraft({
        panelId,
        tool,
        anchor: null,
        at: null,
        surface: surfaceId,
      })
    }
    container.addEventListener("pointermove", onMove, true)
    container.addEventListener("pointerleave", onLeave)
    container.addEventListener("pointerdown", onDown, true)
    window.addEventListener("pointerup", onUp, true)
    window.addEventListener("pointercancel", onCancel, true)
    container.addEventListener("click", onClick, true)
    container.addEventListener("keydown", onKey, true)
    return () => {
      container.removeEventListener("pointermove", onMove, true)
      container.removeEventListener("pointerleave", onLeave)
      container.removeEventListener("pointerdown", onDown, true)
      window.removeEventListener("pointerup", onUp, true)
      window.removeEventListener("pointercancel", onCancel, true)
      container.removeEventListener("click", onClick, true)
      container.removeEventListener("keydown", onKey, true)
    }
  }, [containerRef, surfaceRef, current, panelId, tool, surfaceId])

  return (
    <div
      aria-hidden="true"
      data-canvas-outline={
        hover?.outline ? "target" : hover ? "panel" : undefined
      }
      className="pointer-events-none absolute inset-0 z-10"
    >
      {hover === null ? null : hover.outline === null ? (
        <div className="absolute inset-0 rounded-card outline-2 -outline-offset-2 outline-primary" />
      ) : (
        <div
          className="absolute rounded-chip outline-2 outline-primary"
          style={{
            left: hover.offset.x + hover.outline.x,
            top: hover.offset.y + hover.outline.y,
            width: Math.max(2, hover.outline.width),
            height: Math.max(2, hover.outline.height),
          }}
        />
      )}
    </div>
  )
}
