import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { AtSignIcon, MessageSquarePlusIcon } from "lucide-react"

import { AskPins } from "@/comments/ask-pins"
import {
  blockSurfaceId,
  useCanvasHost,
  type CanvasHost,
} from "@/comments/canvas-host"
import { CanvasPins } from "@/comments/canvas-pins"
import { CanvasSpots } from "@/comments/canvas-spots"
import type { CanvasSurface } from "@/comments/canvas-target"
import { Button } from "@/components/ui/button"
import {
  activeTool,
  useCanvasTool,
  type DraftTool,
} from "@/dashboards/canvas-tools"
import { FindingMarkdown } from "@/findings/finding-markdown"
import type { Point } from "@/panels/anchors"

import { blockTitle } from "./block-rows"
import { useSize } from "./use-size"
import type { Block } from "./queries"
import {
  quoteOf,
  rangeRect,
  textAnchor,
  textAnchors,
  type TextLayout,
} from "./text-quote"

type Offer = { quote: string; at: Point }

const textLayoutType = "observation-text"

function selectionOffer(
  selection: Selection | null,
  root: HTMLElement,
  surface: HTMLElement
): Offer | null {
  if (selection === null || selection.isCollapsed) return null
  if (selection.rangeCount === 0) return null
  const range = selection.getRangeAt(0)
  if (!root.contains(range.commonAncestorContainer)) return null
  const quote = quoteOf(selection.toString())
  if (quote === "") return null
  const rect = rangeRect(range, surface)
  if (rect === null) return null
  return { quote, at: { x: rect.x, y: rect.y + rect.height } }
}

function SelectionActions({
  offer,
  mayAsk,
  onOpen,
}: {
  offer: Offer
  mayAsk: boolean
  onOpen: (tool: DraftTool) => void
}) {
  return (
    <div
      role="toolbar"
      aria-label="Selected text"
      className="absolute z-30 flex items-center gap-0.5 rounded-control border border-border bg-popover p-0.5 text-popover-foreground"
      style={{ left: offer.at.x, top: offer.at.y + 4 }}
      onPointerDown={(event) => event.preventDefault()}
    >
      <Button variant="ghost" size="sm" onClick={() => onOpen("comment")}>
        <MessageSquarePlusIcon aria-hidden="true" />
        Comment
      </Button>
      {mayAsk ? (
        <Button variant="ghost" size="sm" onClick={() => onOpen("ask")}>
          <AtSignIcon aria-hidden="true" />
          Ask
        </Button>
      ) : null}
    </div>
  )
}

function TextSurface({
  host,
  block,
  markdown,
}: {
  host: CanvasHost
  block: Block
  markdown: string
}) {
  const surfaceRef = useRef<HTMLDivElement>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const surfaceId = blockSurfaceId(block.id)
  const body = useSize(surfaceRef)
  const [layout, setLayout] = useState<TextLayout | null>(null)
  const [offer, setOffer] = useState<Offer | null>(null)
  const tool = useCanvasTool((state) => state.tool)
  const mayAsk = host.gate.agentAvailable === true
  const pointing = activeTool(tool, {
    comment: true,
    ask: mayAsk,
    idle: !host.gate.layoutEditing,
  })
  const title = blockTitle(block)

  useLayoutEffect(() => {
    const root = rootRef.current
    if (root !== null) setLayout({ root, markdown })
  }, [markdown])

  useEffect(() => {
    function onSelectionChange() {
      const root = rootRef.current
      const selection = document.getSelection()
      if (root === null || selection === null || selection.isCollapsed) {
        setOffer(null)
        return
      }
      if (!root.contains(selection.anchorNode)) setOffer(null)
    }
    document.addEventListener("selectionchange", onSelectionChange)
    return () =>
      document.removeEventListener("selectionchange", onSelectionChange)
  }, [])

  useEffect(() => {
    if (offer === null) return
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return
      event.preventDefault()
      setOffer(null)
      document.getSelection()?.removeAllRanges()
    }
    document.addEventListener("keydown", onKeyDown, true)
    return () => document.removeEventListener("keydown", onKeyDown, true)
  }, [offer])

  const surface = useMemo<CanvasSurface>(
    () => ({
      module: textAnchors,
      published: layout === null ? null : { type: textLayoutType, layout },
    }),
    [layout]
  )

  function open(chosen: DraftTool, at: Offer) {
    useCanvasTool.getState().openDraft({
      ownerId: block.id,
      tool: chosen,
      anchor: textAnchor(at.quote, at.at, body),
      at: at.at,
      surface: surfaceId,
    })
    setOffer(null)
  }

  function offerSelection() {
    const root = rootRef.current
    const element = surfaceRef.current
    if (root === null || element === null) return
    if (host.gate.layoutEditing) return setOffer(null)
    const next = selectionOffer(document.getSelection(), root, element)
    if (next !== null && pointing !== "select") open(pointing, next)
    else setOffer(next)
  }

  return (
    <>
      <div ref={surfaceRef} data-canvas-surface="" className="relative">
        <div
          ref={rootRef}
          onPointerUp={offerSelection}
          onKeyUp={offerSelection}
        >
          <FindingMarkdown markdown={markdown} />
        </div>
        {offer === null ? null : (
          <SelectionActions
            offer={offer}
            mayAsk={mayAsk}
            onOpen={(chosen) => open(chosen, offer)}
          />
        )}
        <CanvasPins
          subject={host.subject}
          ownerId={block.id}
          panelTitle={title}
          surface={surface}
          body={body}
          view="data"
          gateInput={host.gate}
        />
        <AskPins
          subject={host.subject}
          ownerId={block.id}
          panelTitle={title}
          surfaceId={surfaceId}
          surface={surface}
          body={body}
          view="data"
        />
      </div>
      <CanvasSpots
        owner={{ ownerId: block.id, surfaceId, copy: false }}
        panelTitle={title}
        surfaceRef={surfaceRef}
      />
    </>
  )
}

export function TextCanvas({
  block,
  markdown,
}: {
  block: Block
  markdown: string
}) {
  const host = useCanvasHost()
  if (host === null) return <FindingMarkdown markdown={markdown} />
  return <TextSurface host={host} block={block} markdown={markdown} />
}
