import {
  useId,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react"
import { MaximizeIcon, ZoomInIcon, ZoomOutIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import {
  initialView,
  maxZoom,
  minZoom,
  zoomStep,
  type View,
} from "../nodeGraph/zoom"
import type { PanelRenderer } from "../types"
import { canvasModel, type CanvasLine } from "./canvas-model"
import { arrowEnds, connectionPath, dashArray, lineCap } from "./connections"
import {
  BackgroundImage,
  ContentItem,
  drawsContent,
  hasShape,
  ShapeItem,
  Tooltip,
  type Hover,
} from "./elements"
import { clampView, zoomedView } from "./placement"

type Drag = { x: number; y: number; view: View }

function markerId(scope: string, line: CanvasLine): string {
  return `${scope}-${line.key.replace(/[^A-Za-z0-9_-]/g, "_")}`
}

function Lines({
  lines,
  width,
  height,
}: {
  lines: CanvasLine[]
  width: number
  height: number
}) {
  const scope = useId().replaceAll(":", "")
  return (
    <svg
      aria-hidden="true"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className="absolute inset-0 overflow-visible"
    >
      <defs>
        {lines.map((line) =>
          line.arrow === "none" ? null : (
            <marker
              key={line.key}
              id={markerId(scope, line)}
              viewBox="0 0 10 10"
              refX={9}
              refY={5}
              markerWidth={4}
              markerHeight={4}
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" style={{ fill: line.color }} />
            </marker>
          )
        )}
      </defs>
      {lines.map((line) => {
        const ends = arrowEnds(line.arrow)
        const marker = `url(#${markerId(scope, line)})`
        return (
          <path
            key={line.key}
            d={connectionPath(line.points, line.path)}
            markerStart={ends.start ? marker : undefined}
            markerEnd={ends.end ? marker : undefined}
            style={{
              fill: "none",
              stroke: line.color,
              strokeWidth: line.width,
              strokeDasharray: dashArray(line.lineStyle) ?? undefined,
              strokeLinecap: lineCap(line.lineStyle),
            }}
          />
        )
      })}
    </svg>
  )
}

const CanvasPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const { resolved: theme } = useTheme()
  const body = useMemo(() => ({ width, height }), [width, height])
  const model = useMemo(
    () => canvasModel(frames, fields, display, body),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- text on a fill reads the live theme tokens, so a theme flip must recompute it
    [frames, fields, display, body, theme]
  )
  const { root } = model
  const rootRef = useRef<HTMLDivElement>(null)
  const drag = useRef<Drag | null>(null)
  const [pan, setView] = useState<View>(initialView)
  const [hover, setHover] = useState<Hover | null>(null)
  const view = root.panZoom
    ? clampView(pan, body, root.infinitePan)
    : initialView

  function zoomBy(factor: number) {
    setView(zoomedView(view, factor, body, root.infinitePan))
  }

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    const onControl =
      event.target instanceof Element && event.target.closest("button, a")
    if (event.button !== 0 || onControl) return
    drag.current = { x: event.clientX, y: event.clientY, view }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const start = drag.current
    if (!start) return
    setView(
      clampView(
        {
          zoom: start.view.zoom,
          panX: start.view.panX + event.clientX - start.x,
          panY: start.view.panY + event.clientY - start.y,
        },
        body,
        root.infinitePan
      )
    )
  }

  function showHover(
    details: Hover["details"],
    clientX: number,
    clientY: number
  ) {
    const origin = rootRef.current?.getBoundingClientRect()
    if (!origin) return
    setHover({ details, x: clientX - origin.left, y: clientY - origin.top })
  }

  const shaped = model.elements.filter(hasShape)
  const drawn = model.elements.filter((element) =>
    drawsContent(element, root.tooltip)
  )
  return (
    <div
      ref={rootRef}
      role="group"
      aria-label={panel.title}
      data-elements={model.elements.length}
      className="relative min-h-0 min-w-0 overflow-hidden text-foreground"
      style={{
        width,
        height,
        backgroundColor: root.backgroundColor ?? undefined,
        borderRadius: root.borderRadius,
      }}
    >
      <div
        className={cn(
          "absolute inset-0",
          root.panZoom && "cursor-grab touch-none active:cursor-grabbing"
        )}
        onPointerDown={root.panZoom ? onPointerDown : undefined}
        onPointerMove={root.panZoom ? onPointerMove : undefined}
        onPointerUp={() => (drag.current = null)}
        onPointerCancel={() => (drag.current = null)}
      >
        <div
          data-canvas-stage=""
          className="absolute inset-0"
          style={
            root.panZoom
              ? {
                  transform: `translate(${view.panX}px, ${view.panY}px) scale(${view.zoom})`,
                }
              : undefined
          }
        >
          {root.background ? (
            <BackgroundImage background={root.background} />
          ) : null}
          {shaped.map((element) => (
            <ShapeItem key={element.key} element={element} />
          ))}
          <Lines lines={model.lines} width={width} height={height} />
          {drawn.map((element) => (
            <ContentItem
              key={element.key}
              element={element}
              tooltipOn={root.tooltip}
              onShow={showHover}
              onHide={() => setHover(null)}
            />
          ))}
        </div>
      </div>
      {root.borderWidth > 0 ? (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 border-solid"
          style={{
            borderWidth: root.borderWidth,
            borderColor: root.borderColor ?? "var(--border)",
            borderRadius: root.borderRadius,
          }}
        />
      ) : null}
      {root.panZoom ? (
        <div
          role="toolbar"
          aria-label="Zoom"
          className="absolute top-2 left-2 flex gap-1"
        >
          <Button
            variant="outline"
            size="icon-xs"
            aria-label="Zoom in"
            disabled={view.zoom >= maxZoom}
            onClick={() => zoomBy(zoomStep)}
          >
            <ZoomInIcon />
          </Button>
          <Button
            variant="outline"
            size="icon-xs"
            aria-label="Zoom out"
            disabled={view.zoom <= minZoom}
            onClick={() => zoomBy(1 / zoomStep)}
          >
            <ZoomOutIcon />
          </Button>
          <Button
            variant="outline"
            size="icon-xs"
            aria-label="Reset zoom"
            onClick={() => setView(initialView)}
          >
            <MaximizeIcon />
          </Button>
        </div>
      ) : null}
      {root.tooltip && hover ? (
        <Tooltip hover={hover} width={width} height={height} />
      ) : null}
    </div>
  )
}

export default CanvasPanel
