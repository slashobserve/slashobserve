import { MinusIcon, PlusIcon, RotateCcwIcon } from "lucide-react"
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react"

import { Button } from "@/components/ui/button"

import type { PanelRenderer } from "../types"
import { thresholdIcons } from "../thresholds"
import {
  geoModel,
  initialView,
  skippedText,
  truncatedText,
  viewOptions,
  type Marker,
} from "./geo-model"
import {
  clampCenter,
  clampZoom,
  maxZoom,
  minZoom,
  panBy,
  worldSize,
  type View,
} from "./projection"
import { worldOutline } from "./world"

const tooltipOffset = 12
const tooltipHeight = 120
const panStep = 0.1
const keyboardHint =
  "Arrow keys pan, plus and minus zoom, 0 resets the view. View as table lists every marker."
const panKeys: Record<string, [number, number]> = {
  ArrowLeft: [-panStep, 0],
  ArrowRight: [panStep, 0],
  ArrowUp: [0, -panStep],
  ArrowDown: [0, panStep],
}

type Hover = { marker: Marker; x: number; y: number }
type Drag = { startX: number; startY: number; view: View }

function Tooltip({
  hover,
  width,
  height,
}: {
  hover: Hover
  width: number
  height: number
}) {
  const { marker } = hover
  const onRight = hover.x > width / 2
  const Icon = marker.state ? thresholdIcons[marker.state.level] : null
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 max-w-64 min-w-36 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(
          hover.y + tooltipOffset,
          Math.max(0, height - tooltipHeight)
        ),
        ...(onRight
          ? { right: width - hover.x + tooltipOffset }
          : { left: hover.x + tooltipOffset }),
      }}
    >
      <div className="mb-1 text-muted-foreground tabular-nums">
        {marker.label}
      </div>
      {marker.details.map((detail) => (
        <div key={detail.name} className="flex justify-between gap-3">
          <span className="min-w-0 truncate text-muted-foreground">
            {detail.name}
          </span>
          <span className="min-w-0 truncate font-medium tabular-nums">
            {detail.text}
          </span>
        </div>
      ))}
      {Icon && marker.state ? (
        <div className="mt-1 flex items-center gap-1">
          <Icon
            className="size-3 shrink-0"
            strokeWidth={2.4}
            aria-hidden="true"
            style={{ color: marker.color }}
          />
          <span>{marker.state.word}</span>
        </div>
      ) : null}
    </div>
  )
}

function useOutline(): string {
  const [outline, setOutline] = useState("")
  useEffect(() => {
    let live = true
    worldOutline()
      .then((path) => {
        if (live) setOutline(path)
      })
      .catch(() => undefined)
    return () => {
      live = false
    }
  }, [])
  return outline
}

export const GeomapPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const model = useMemo(
    () => geoModel(frames, fields, display),
    [frames, fields, display]
  )
  const options = useMemo(() => viewOptions(display), [display])
  const initial = useMemo(
    () => initialView(options, model.markers, width, height),
    [options, model.markers, width, height]
  )
  const outline = useOutline()
  const [manual, setManual] = useState<View | null>(null)
  const [hover, setHover] = useState<Hover | null>(null)
  const drag = useRef<Drag | null>(null)
  const frame = useRef<HTMLDivElement>(null)
  const hintId = useId()

  if (model.reason) {
    return (
      <div
        className="flex items-center justify-center p-4 text-center text-sm text-muted-foreground"
        style={{ width, height }}
      >
        {model.reason}
      </div>
    )
  }

  const view = manual ?? initial
  const size = worldSize(view.zoom)
  const left = width / 2 - view.center.x * size
  const top = height / 2 - view.center.y * size
  const visible = model.markers.filter((marker) => {
    const x = left + marker.x * size
    const y = top + marker.y * size
    return (
      x >= -marker.radius &&
      x <= width + marker.radius &&
      y >= -marker.radius &&
      y <= height + marker.radius
    )
  })
  const notes = [
    model.skipped > 0 ? skippedText(model.skipped) : null,
    model.truncated > 0 ? truncatedText(model.truncated) : null,
  ].filter((note) => note !== null)

  function zoomBy(step: number) {
    setHover(null)
    setManual({
      center: view.center,
      zoom: clampZoom(view.zoom + step, width, height),
    })
  }

  function resetView() {
    setHover(null)
    setManual(null)
  }

  function onKeyDown(event: ReactKeyboardEvent<SVGSVGElement>) {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    const pan = panKeys[event.key]
    if (pan) {
      setHover(null)
      setManual(panBy(view, pan[0], pan[1], width, height))
    } else if (event.key === "+" || event.key === "=") zoomBy(1)
    else if (event.key === "-") zoomBy(-1)
    else if (event.key === "0") resetView()
    else return
    event.preventDefault()
  }

  function onPointerDown(event: ReactPointerEvent<SVGSVGElement>) {
    drag.current = {
      startX: event.clientX,
      startY: event.clientY,
      view,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
    setHover(null)
  }

  function onPointerMove(event: ReactPointerEvent<SVGSVGElement>) {
    const active = drag.current
    if (!active) return
    const scale = worldSize(active.view.zoom)
    setManual({
      zoom: active.view.zoom,
      center: {
        x: clampCenter(
          active.view.center.x - (event.clientX - active.startX) / scale
        ),
        y: clampCenter(
          active.view.center.y - (event.clientY - active.startY) / scale
        ),
      },
    })
  }

  function onPointerUp(event: ReactPointerEvent<SVGSVGElement>) {
    drag.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  function showHover(
    marker: Marker,
    event: ReactPointerEvent<SVGCircleElement>
  ) {
    if (drag.current || !frame.current) return
    const box = frame.current.getBoundingClientRect()
    setHover({
      marker,
      x: event.clientX - box.left,
      y: event.clientY - box.top,
    })
  }

  return (
    <div
      ref={frame}
      data-markers={model.markers.length}
      data-view={options.mode}
      className="relative overflow-hidden"
      style={{ width, height }}
    >
      <svg
        tabIndex={0}
        role="group"
        aria-roledescription="map"
        aria-label={`Map with ${model.markers.length} markers`}
        aria-describedby={hintId}
        width={width}
        height={height}
        className="block cursor-grab touch-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring active:cursor-grabbing"
        onKeyDown={onKeyDown}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <path
          d={outline}
          transform={`translate(${left} ${top}) scale(${size})`}
          vectorEffect="non-scaling-stroke"
          style={{ fill: "none", stroke: "var(--border)", strokeWidth: 1 }}
        />
        {visible.map((marker) => (
          <circle
            key={marker.key}
            cx={left + marker.x * size}
            cy={top + marker.y * size}
            r={marker.radius}
            onPointerEnter={(event) => showHover(marker, event)}
            onPointerMove={(event) => showHover(marker, event)}
            onPointerLeave={() => setHover(null)}
            style={{
              fill: marker.color,
              fillOpacity: 0.8,
              stroke: "var(--background)",
              strokeWidth: 1,
            }}
          />
        ))}
      </svg>
      <div className="absolute top-2 left-2 flex flex-col gap-1">
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="Zoom in"
          disabled={view.zoom >= maxZoom}
          onClick={() => zoomBy(1)}
        >
          <PlusIcon />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="Zoom out"
          disabled={view.zoom <= minZoom(width, height)}
          onClick={() => zoomBy(-1)}
        >
          <MinusIcon />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="Reset view"
          disabled={manual === null}
          onClick={resetView}
        >
          <RotateCcwIcon />
        </Button>
      </div>
      <span id={hintId} className="sr-only">
        {keyboardHint}
      </span>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-between gap-3 px-2 pb-1 text-xs text-muted-foreground">
        <span className="min-w-0 truncate">{notes.join(" ")}</span>
        <span className="shrink-0">Natural Earth</span>
      </div>
      {hover ? <Tooltip hover={hover} width={width} height={height} /> : null}
    </div>
  )
}

export default GeomapPanel
