import { useQuery } from "@tanstack/react-query"
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react"

import { assetImageUrl } from "@/lib/remote-assets"
import { useTheme } from "@/lib/theme"
import { outboundDomainsQuery } from "@/settings/outbound-domains"

import type { PanelRenderer } from "../types"
import { thresholdIcons } from "../thresholds"
import {
  basemapOf,
  outlineAttribution,
  tileAccess,
  tileUrl,
  type TileSource,
} from "./basemaps"
import {
  BlockedTilesNote,
  DebugLine,
  MeasureReadout,
  MeasureToggle,
  ScaleBars,
  ZoomControls,
} from "./controls"
import {
  fitChoice,
  fitPoints,
  geoModel,
  initialView,
  skippedText,
  symbolPath,
  truncatedText,
  viewOptions,
  viewTuning,
  type Marker,
  type MarkersLayerModel,
} from "./geo-model"
import {
  clampCenter,
  clampZoom,
  panBy,
  project,
  unproject,
  worldSize,
  zoomAround,
  zoomBounds,
  type LonLat,
  type View,
} from "./projection"
import { pathMeters } from "./scale"
import { useManualView } from "./shared-view"
import { visibleTiles } from "./tiles"
import { worldOutline } from "./world"

const tooltipOffset = 12
const tooltipHeight = 120
const panStep = 0.1
const clickSlop = 4
const seamOverlap = 0.5
const wheelPixelStep = 0.002
const wheelLineStep = 0.05
const wheelMaxStep = 1
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

function useOutline(enabled: boolean): string {
  const [outline, setOutline] = useState("")
  useEffect(() => {
    if (!enabled) return
    let live = true
    worldOutline()
      .then((path) => {
        if (live) setOutline(path)
      })
      .catch(() => undefined)
    return () => {
      live = false
    }
  }, [enabled])
  return outline
}

function TileLayer({
  source,
  view,
  width,
  height,
  opacity,
}: {
  source: TileSource
  view: View
  width: number
  height: number
  opacity: number
}) {
  const tiles = visibleTiles(view, width, height)
  return (
    <g opacity={opacity} aria-hidden="true">
      {tiles.map((tile) => {
        const href = assetImageUrl(tileUrl(source, tile))
        if (href === null) return null
        return (
          <image
            key={`${tile.z}/${tile.column}/${tile.y}`}
            href={href}
            x={tile.left}
            y={tile.top}
            width={tile.size + seamOverlap}
            height={tile.size + seamOverlap}
            preserveAspectRatio="none"
          />
        )
      })}
    </g>
  )
}

function MarkersLayer({
  layer,
  left,
  top,
  size,
  width,
  height,
  tooltipOn,
  onHover,
  onLeave,
}: {
  layer: MarkersLayerModel
  left: number
  top: number
  size: number
  width: number
  height: number
  tooltipOn: boolean
  onHover: (marker: Marker, event: ReactPointerEvent<SVGElement>) => void
  onLeave: () => void
}) {
  const hoverable = tooltipOn && layer.tooltip
  return (
    <g>
      {layer.markers.flatMap((marker) => {
        const x = left + marker.x * size
        const y = top + marker.y * size
        const reach = marker.radius + layer.textSize * 2
        if (x < -reach || x > width + reach) return []
        if (y < -reach || y > height + reach) return []
        return [
          <g key={marker.key}>
            <path
              d={symbolPath(marker.symbol, marker.radius)}
              transform={`translate(${x} ${y}) rotate(${marker.rotation})`}
              onPointerEnter={hoverable ? (e) => onHover(marker, e) : undefined}
              onPointerMove={hoverable ? (e) => onHover(marker, e) : undefined}
              onPointerLeave={hoverable ? onLeave : undefined}
              style={{
                fill: marker.color,
                fillOpacity: 0.8 * layer.opacity,
                stroke: "var(--background)",
                strokeWidth: 1,
                strokeOpacity: layer.opacity,
              }}
            />
            {marker.text === null ? null : (
              <text
                x={x}
                y={y + marker.radius + layer.textSize}
                textAnchor="middle"
                fontSize={layer.textSize}
                pointerEvents="none"
                style={{
                  fill: "var(--foreground)",
                  stroke: "var(--background)",
                  strokeWidth: 3,
                  paintOrder: "stroke",
                  opacity: layer.opacity,
                }}
              >
                {marker.text}
              </text>
            )}
          </g>,
        ]
      })}
    </g>
  )
}

function unique(values: readonly string[]): string[] {
  return [...new Set(values)]
}

const GeomapPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
  stateScope,
}) => {
  const { resolved: theme } = useTheme()
  const model = useMemo(
    () => geoModel(frames, fields, display, theme),
    [frames, fields, display, theme]
  )
  const basemap = useMemo(() => basemapOf(display, theme), [display, theme])
  const options = useMemo(() => viewOptions(display), [display])
  const tuning = useMemo(() => viewTuning(display), [display])
  const points = useMemo(
    () => fitPoints(model, fitChoice(display)),
    [model, display]
  )
  const initial = useMemo(
    () => initialView(options, points, width, height, tuning),
    [options, points, width, height, tuning]
  )
  const tileSources = [
    ...(basemap.kind === "tiles" ? [basemap.source] : []),
    ...model.layers.flatMap((layer) =>
      layer.kind === "tiles" ? [layer.source] : []
    ),
  ]
  const domains = useQuery({
    ...outboundDomainsQuery(),
    enabled: tileSources.length > 0,
    staleTime: 60_000,
  })
  const patterns = useMemo(
    () => domains.data?.domains.map((entry) => entry.domain),
    [domains.data]
  )
  const accessOf = (source: TileSource) => tileAccess(source, patterns)
  const blocked = tileSources
    .map(accessOf)
    .find((access) => access.state === "blocked")
  const basemapTiles =
    basemap.kind === "tiles" && accessOf(basemap.source).state === "allowed"
      ? basemap.source
      : null
  const drawOutline =
    basemap.kind === "outline" ||
    basemap.kind === "unusable" ||
    (basemap.kind === "tiles" && basemapTiles === null)
  const outline = useOutline(drawOutline)
  const [manual, setManual] = useManualView(
    stateScope,
    display.sharedView === true
  )
  const [hover, setHover] = useState<Hover | null>(null)
  const [measuring, setMeasuring] = useState(false)
  const [route, setRoute] = useState<LonLat[]>([])
  const drag = useRef<Drag | null>(null)
  const frame = useRef<HTMLDivElement>(null)
  const surface = useRef<SVGSVGElement>(null)
  const hintId = useId()

  const view = manual ?? initial
  const size = worldSize(view.zoom)
  const left = width / 2 - view.center.x * size
  const top = height / 2 - view.center.y * size
  const wheelZoom = display.mouseWheelZoom === true

  useEffect(() => {
    const element = surface.current
    if (!element || !wheelZoom) return
    const onWheel = (event: WheelEvent) => {
      const step = event.deltaMode === 1 ? wheelLineStep : wheelPixelStep
      const delta = Math.max(
        -wheelMaxStep,
        Math.min(wheelMaxStep, -event.deltaY * step)
      )
      const box = element.getBoundingClientRect()
      const next = zoomAround(
        view,
        view.zoom + delta,
        event.clientX - box.left,
        event.clientY - box.top,
        width,
        height,
        tuning.limits
      )
      if (next.zoom === view.zoom) return
      event.preventDefault()
      setHover(null)
      setManual(next)
    }
    element.addEventListener("wheel", onWheel, { passive: false })
    return () => element.removeEventListener("wheel", onWheel)
  }, [wheelZoom, view, width, height, tuning, setManual])

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

  const bounds = zoomBounds(width, height, tuning.limits)
  const tileLayers = model.layers.flatMap((layer) =>
    layer.kind === "tiles" && accessOf(layer.source).state === "allowed"
      ? [layer]
      : []
  )
  const notes = [
    ...model.notes,
    basemap.kind === "unusable" ? basemap.reason : null,
    model.skipped > 0 ? skippedText(model.skipped) : null,
    model.truncated > 0 ? truncatedText(model.truncated) : null,
  ].filter((note) => note !== null)
  const attributions = unique([
    ...(drawOutline ? [outlineAttribution] : []),
    ...(basemapTiles ? [basemapTiles.attribution] : []),
    ...tileLayers.map((layer) => layer.source.attribution),
  ])
  const tooltipOn = display.tooltip !== "none"
  const centre = unproject(view.center.x, view.center.y)

  function zoomBy(step: number) {
    setHover(null)
    setManual({
      center: view.center,
      zoom: clampZoom(view.zoom + step, width, height, tuning.limits),
    })
  }

  function resetView() {
    setHover(null)
    setManual(null)
  }

  function stopMeasuring() {
    setMeasuring(false)
    setRoute([])
  }

  function pointAt(clientX: number, clientY: number): LonLat {
    const box = surface.current?.getBoundingClientRect()
    const x = view.center.x + (clientX - (box?.left ?? 0) - width / 2) / size
    const y = view.center.y + (clientY - (box?.top ?? 0) - height / 2) / size
    return unproject(clampCenter(x), clampCenter(y))
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
    else if (measuring && event.key === "Enter") {
      setRoute((current) => [
        ...current,
        unproject(view.center.x, view.center.y),
      ])
    } else if (measuring && event.key === "Escape") stopMeasuring()
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
    const active = drag.current
    drag.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
    if (!measuring || !active) return
    const moved = Math.hypot(
      event.clientX - active.startX,
      event.clientY - active.startY
    )
    if (moved >= clickSlop) return
    const point = pointAt(event.clientX, event.clientY)
    setRoute((current) => [...current, point])
  }

  function onPointerCancel(event: ReactPointerEvent<SVGSVGElement>) {
    drag.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  function showHover(marker: Marker, event: ReactPointerEvent<SVGElement>) {
    if (drag.current || !frame.current) return
    const box = frame.current.getBoundingClientRect()
    setHover({
      marker,
      x: event.clientX - box.left,
      y: event.clientY - box.top,
    })
  }

  const routePixels = route.map(({ lon, lat }) => {
    const { x, y } = project(lon, lat)
    return { x: left + x * size, y: top + y * size }
  })

  return (
    <div
      ref={frame}
      data-markers={model.markers.length}
      data-view={options.mode}
      className="relative overflow-hidden"
      style={{ width, height }}
    >
      <svg
        ref={surface}
        tabIndex={0}
        role="group"
        aria-roledescription="map"
        aria-label={`Map with ${model.markers.length} markers`}
        aria-describedby={hintId}
        width={width}
        height={height}
        className={`block touch-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring ${
          measuring ? "cursor-crosshair" : "cursor-grab active:cursor-grabbing"
        }`}
        onKeyDown={onKeyDown}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerCancel}
      >
        {basemapTiles ? (
          <TileLayer
            source={basemapTiles}
            view={view}
            width={width}
            height={height}
            opacity={
              typeof display.basemapOpacity === "number"
                ? display.basemapOpacity
                : 1
            }
          />
        ) : null}
        {drawOutline ? (
          <path
            d={outline}
            transform={`translate(${left} ${top}) scale(${size})`}
            vectorEffect="non-scaling-stroke"
            style={{ fill: "none", stroke: "var(--border)", strokeWidth: 1 }}
          />
        ) : null}
        {model.layers.map((layer, index) =>
          layer.kind === "tiles" ? (
            tileLayers.includes(layer) ? (
              <TileLayer
                key={index}
                source={layer.source}
                view={view}
                width={width}
                height={height}
                opacity={layer.opacity}
              />
            ) : null
          ) : (
            <MarkersLayer
              key={index}
              layer={layer}
              left={left}
              top={top}
              size={size}
              width={width}
              height={height}
              tooltipOn={tooltipOn}
              onHover={showHover}
              onLeave={() => setHover(null)}
            />
          )
        )}
        {routePixels.length > 0 ? (
          <g pointerEvents="none">
            {routePixels.length > 1 ? (
              <polyline
                points={routePixels.map(({ x, y }) => `${x},${y}`).join(" ")}
                fill="none"
                style={{ stroke: "var(--foreground)", strokeWidth: 2 }}
              />
            ) : null}
            {routePixels.map(({ x, y }, index) => (
              <circle
                key={index}
                cx={x}
                cy={y}
                r={4}
                style={{
                  fill: "var(--background)",
                  stroke: "var(--foreground)",
                  strokeWidth: 2,
                }}
              />
            ))}
          </g>
        ) : null}
      </svg>
      <div className="absolute top-2 left-2 flex flex-col gap-1">
        {display.showZoom === false ? null : (
          <ZoomControls
            canZoomIn={view.zoom < bounds.highest}
            canZoomOut={view.zoom > bounds.lowest}
            canReset={manual !== null}
            onZoomIn={() => zoomBy(1)}
            onZoomOut={() => zoomBy(-1)}
            onReset={resetView}
          />
        )}
        {display.showMeasure === true ? (
          <MeasureToggle
            active={measuring}
            onToggle={() => (measuring ? stopMeasuring() : setMeasuring(true))}
          />
        ) : null}
      </div>
      {measuring ? (
        <MeasureReadout points={route.length} meters={pathMeters(route)} />
      ) : null}
      {display.showScale === true ? (
        <ScaleBars latitude={centre.lat} zoom={view.zoom} />
      ) : null}
      <span id={hintId} className="sr-only">
        {keyboardHint}
      </span>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-between gap-3 px-2 pb-1 text-xs text-muted-foreground">
        <span className="min-w-0 truncate">
          {notes.join(" ")}
          {blocked?.state === "blocked" ? (
            <>
              {notes.length > 0 ? " " : null}
              <BlockedTilesNote host={blocked.host} />
            </>
          ) : null}
        </span>
        <span className="flex shrink-0 gap-3">
          {display.showDebug === true ? (
            <DebugLine
              zoom={view.zoom}
              latitude={centre.lat}
              longitude={centre.lon}
            />
          ) : null}
          {display.showAttribution === false ? null : (
            <span>{attributions.join(" · ")}</span>
          )}
        </span>
      </div>
      {hover ? <Tooltip hover={hover} width={width} height={height} /> : null}
    </div>
  )
}

export default GeomapPanel
