import { useMemo, useRef, type ReactNode } from "react"

import { CanvasLayer } from "@/comments/canvas-composer"
import {
  blockSurfaceId,
  useCanvasHost,
  type CanvasHost,
} from "@/comments/canvas-host"
import { CanvasSpots } from "@/comments/canvas-spots"
import { activeTool, useCanvasTool } from "@/dashboards/canvas-tools"
import { createAnchorLayoutStore } from "@/panels/anchor-layout"

import { blockTitle } from "./block-rows"
import type { Block } from "./queries"
import { useSize } from "./use-size"

const noRange = { from: "", to: "" }

function WholeBlockSurface({
  host,
  block,
  children,
}: {
  host: CanvasHost
  block: Block
  children: ReactNode
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const store = useMemo(() => createAnchorLayoutStore(), [])
  const body = useSize(surfaceRef)
  const tool = useCanvasTool((state) => state.tool)
  const pointing = activeTool(tool, {
    comment: true,
    ask: host.gate.agentAvailable === true,
    idle: !host.gate.layoutEditing,
  })
  const surfaceId = blockSurfaceId(block.id)
  return (
    <>
      <div ref={containerRef} className="relative">
        <div ref={surfaceRef} data-canvas-surface="">
          {children}
        </div>
        {pointing === "select" ? null : (
          <CanvasLayer
            containerRef={containerRef}
            surfaceRef={surfaceRef}
            store={store}
            surfaceId={surfaceId}
            ownerId={block.id}
            tool={pointing}
            body={body}
            shownRange={noRange}
            wholePanel
          />
        )}
      </div>
      <CanvasSpots
        owner={{ ownerId: block.id, surfaceId, copy: false }}
        panelTitle={blockTitle(block)}
        surfaceRef={surfaceRef}
      />
    </>
  )
}

export function WholeBlockCanvas({
  block,
  onCanvas,
  children,
}: {
  block: Block
  onCanvas: boolean
  children: ReactNode
}) {
  const host = useCanvasHost()
  if (!onCanvas || host === null) return children
  return (
    <WholeBlockSurface host={host} block={block}>
      {children}
    </WholeBlockSurface>
  )
}
