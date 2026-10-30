import { Link } from "@tanstack/react-router"
import {
  useEffect,
  useId,
  useRef,
  type PointerEvent as ReactPointerEvent,
} from "react"

import type { ResolvedTheme } from "@/lib/theme"

import { resolveColor } from "../../colors"
import { cssColorToRgb } from "../../contrast"
import type {
  DayNightLayerModel,
  GeojsonLayerModel,
  HeatmapLayerModel,
  NetworkLayerModel,
  PhotosLayerModel,
  RouteLayerModel,
  Tip,
} from "../geo-model"
import type { Arrow } from "./route"
import type { Bounds } from "./geojson"
import { photoSize } from "./photos"

export type Box = {
  left: number
  top: number
  size: number
  width: number
  height: number
}

type Hovering = {
  hoverable: boolean
  onHover: (tip: Tip, event: ReactPointerEvent<SVGElement>) => void
  onLeave: () => void
}

const heatPeakAlpha = 0.6
const cullMargin = 8
const nightOpacity = 0.15
const sunRadius = 6
const dotRadius = 4
const arrowViewBox = "0 0 10 10"
const arrowShape = "M0 0L10 5L0 10Z"

function place(box: Box): string {
  return `translate(${box.left} ${box.top}) scale(${box.size})`
}

function onScreen(bounds: Bounds, box: Box): boolean {
  const { left, top, size, width, height } = box
  return !(
    left + bounds.maxX * size < -cullMargin ||
    left + bounds.minX * size > width + cullMargin ||
    top + bounds.maxY * size < -cullMargin ||
    top + bounds.minY * size > height + cullMargin
  )
}

function pointOnScreen(x: number, y: number, box: Box, reach: number): boolean {
  return (
    x >= -reach &&
    x <= box.width + reach &&
    y >= -reach &&
    y <= box.height + reach
  )
}

function hoverProps(
  tip: Tip,
  { hoverable, onHover, onLeave }: Hovering
): {
  onPointerEnter?: (event: ReactPointerEvent<SVGElement>) => void
  onPointerMove?: (event: ReactPointerEvent<SVGElement>) => void
  onPointerLeave?: () => void
} {
  if (!hoverable) return {}
  return {
    onPointerEnter: (event) => onHover(tip, event),
    onPointerMove: (event) => onHover(tip, event),
    onPointerLeave: onLeave,
  }
}

function paintHeat(
  canvas: HTMLCanvasElement,
  layer: HeatmapLayerModel,
  box: Box
) {
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.round(box.width * ratio)
  canvas.height = Math.round(box.height * ratio)
  canvas.style.width = `${box.width}px`
  canvas.style.height = `${box.height}px`
  const context = canvas.getContext("2d")
  const rgb = cssColorToRgb(resolveColor("var(--series-1)"))
  if (!context || !rgb) return
  context.scale(ratio, ratio)
  const reach = layer.radius + layer.blur
  const core = reach === 0 ? 1 : layer.radius / reach
  const [red, green, blue] = rgb
  const stop = (alpha: number) => `rgba(${red}, ${green}, ${blue}, ${alpha})`
  for (const point of layer.points) {
    if (point.weight <= 0) continue
    const x = box.left + point.x * box.size
    const y = box.top + point.y * box.size
    if (!pointOnScreen(x, y, box, reach)) continue
    const alpha = heatPeakAlpha * point.weight
    const gradient = context.createRadialGradient(x, y, 0, x, y, reach)
    gradient.addColorStop(0, stop(alpha))
    gradient.addColorStop(core, stop(alpha))
    gradient.addColorStop(1, stop(0))
    context.fillStyle = gradient
    context.fillRect(x - reach, y - reach, 2 * reach, 2 * reach)
  }
}

export function HeatmapLayer({
  layer,
  box,
  theme,
}: {
  layer: HeatmapLayerModel
  box: Box
  theme: ResolvedTheme
}) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const { left, top, size, width, height } = box
  useEffect(() => {
    const element = canvas.current
    if (!element) return
    const frame = requestAnimationFrame(() =>
      paintHeat(element, layer, { left, top, size, width, height })
    )
    return () => cancelAnimationFrame(frame)
  }, [layer, left, top, size, width, height, theme])
  return (
    <foreignObject
      x={0}
      y={0}
      width={width}
      height={height}
      opacity={layer.opacity}
      pointerEvents="none"
    >
      <canvas ref={canvas} aria-hidden="true" className="block" />
    </foreignObject>
  )
}

export function GeojsonLayer({
  layer,
  box,
  ...hovering
}: { layer: GeojsonLayerModel; box: Box } & Hovering) {
  const interactive = hovering.hoverable && layer.tooltip
  return (
    <g opacity={layer.opacity}>
      {layer.features.map((feature) => {
        if (!onScreen(feature.bounds, box)) return null
        const { style } = feature
        const events = hoverProps(feature.tip, {
          ...hovering,
          hoverable: interactive,
        })
        return (
          <g key={feature.key}>
            {feature.path === "" ? null : (
              <path
                d={feature.path}
                transform={place(box)}
                vectorEffect="non-scaling-stroke"
                fillRule="evenodd"
                pointerEvents={interactive ? "all" : "none"}
                style={{
                  fill: style.fill ?? "none",
                  fillOpacity: style.fillOpacity,
                  stroke: style.stroke,
                  strokeOpacity: style.strokeOpacity,
                  strokeWidth: 1,
                }}
                {...events}
              />
            )}
            {feature.dots.map((dot, index) => (
              <circle
                key={index}
                cx={box.left + dot.x * box.size}
                cy={box.top + dot.y * box.size}
                r={dotRadius}
                pointerEvents={interactive ? "all" : "none"}
                style={{
                  fill: style.fill ?? style.stroke,
                  fillOpacity: style.fill === null ? style.strokeOpacity : 1,
                  stroke: "var(--background)",
                  strokeWidth: 1,
                }}
                {...events}
              />
            ))}
          </g>
        )
      })}
    </g>
  )
}

export function DayNightLayer({
  layer,
  box,
}: {
  layer: DayNightLayerModel
  box: Box
}) {
  return (
    <g opacity={layer.opacity} pointerEvents="none" aria-hidden="true">
      <path
        d={layer.path}
        transform={place(box)}
        style={{
          fill: layer.color ?? "var(--foreground)",
          fillOpacity: nightOpacity,
        }}
      />
      {layer.sun ? (
        <circle
          cx={box.left + layer.sun.x * box.size}
          cy={box.top + layer.sun.y * box.size}
          r={sunRadius}
          style={{
            fill: "var(--threshold-warning)",
            stroke: "var(--background)",
            strokeWidth: 2,
          }}
        />
      ) : null}
    </g>
  )
}

function ArrowHead({
  id,
  color,
  lineWidth,
}: {
  id: string
  color: string
  lineWidth: number
}) {
  const length = 4 + 2 * lineWidth
  return (
    <marker
      id={id}
      viewBox={arrowViewBox}
      refX={8}
      refY={5}
      markerWidth={length}
      markerHeight={length}
      markerUnits="userSpaceOnUse"
      orient="auto-start-reverse"
    >
      <path d={arrowShape} style={{ fill: color }} />
    </marker>
  )
}

function arrowEnds(arrow: Arrow, id: string) {
  return {
    markerStart:
      arrow === "reverse" || arrow === "both" ? `url(#${id})` : undefined,
    markerEnd:
      arrow === "forward" || arrow === "both" ? `url(#${id})` : undefined,
  }
}

export function RouteLayer({
  layer,
  box,
  ...hovering
}: { layer: RouteLayerModel; box: Box } & Hovering) {
  const base = useId()
  const interactive = hovering.hoverable && layer.tooltip
  return (
    <g opacity={layer.opacity}>
      {layer.lines.map((line) => {
        const id = `${base}-${line.key}`
        const pixels = line.points.map((point) => ({
          point,
          x: box.left + point.x * box.size,
          y: box.top + point.y * box.size,
        }))
        return (
          <g key={line.key}>
            {layer.arrow === "none" ? null : (
              <defs>
                <ArrowHead
                  id={id}
                  color={line.color}
                  lineWidth={layer.lineWidth}
                />
              </defs>
            )}
            <polyline
              points={pixels.map(({ x, y }) => `${x},${y}`).join(" ")}
              fill="none"
              pointerEvents="none"
              strokeLinejoin="round"
              strokeLinecap="round"
              style={{ stroke: line.color, strokeWidth: layer.lineWidth }}
              {...arrowEnds(layer.arrow, id)}
            />
            {pixels.map(({ point, x, y }) =>
              pointOnScreen(x, y, box, layer.lineWidth + dotRadius) ? (
                <circle
                  key={point.key}
                  cx={x}
                  cy={y}
                  r={layer.lineWidth + 1}
                  pointerEvents={interactive ? "all" : "none"}
                  style={{
                    fill: line.color,
                    stroke: "var(--background)",
                    strokeWidth: 1,
                  }}
                  {...hoverProps(point.tip, {
                    ...hovering,
                    hoverable: interactive,
                  })}
                />
              ) : null
            )}
          </g>
        )
      })}
    </g>
  )
}

export function NetworkEdges({
  layer,
  box,
}: {
  layer: NetworkLayerModel
  box: Box
}) {
  const id = useId()
  const color = layer.color ?? "var(--muted-foreground)"
  return (
    <g opacity={layer.opacity} pointerEvents="none" aria-hidden="true">
      {layer.arrow === "none" ? null : (
        <defs>
          <ArrowHead id={id} color={color} lineWidth={layer.lineWidth} />
        </defs>
      )}
      {layer.edges.map((edge) => (
        <line
          key={edge.key}
          x1={box.left + edge.from.x * box.size}
          y1={box.top + edge.from.y * box.size}
          x2={box.left + edge.to.x * box.size}
          y2={box.top + edge.to.y * box.size}
          style={{ stroke: color, strokeWidth: layer.lineWidth }}
          {...arrowEnds(layer.arrow, id)}
        />
      ))}
    </g>
  )
}

export function PhotosLayer({
  layer,
  box,
  ...hovering
}: { layer: PhotosLayerModel; box: Box } & Hovering) {
  const clip = useId()
  const interactive = hovering.hoverable && layer.tooltip
  const half = photoSize / 2
  const circle = layer.shape === "circle"
  const framed = half + layer.border
  return (
    <g opacity={layer.opacity}>
      <defs>
        <clipPath id={clip}>
          {circle ? (
            <circle r={half} />
          ) : (
            <rect x={-half} y={-half} width={photoSize} height={photoSize} />
          )}
        </clipPath>
      </defs>
      {layer.photos.map((photo) => {
        const x = box.left + photo.x * box.size
        const y = box.top + photo.y * box.size
        if (!pointOnScreen(x, y, box, framed)) return null
        return (
          <g
            key={photo.key}
            transform={`translate(${x} ${y})`}
            pointerEvents={interactive ? "all" : "none"}
            {...hoverProps(photo.tip, { ...hovering, hoverable: interactive })}
          >
            {layer.border === 0 ? null : circle ? (
              <circle r={framed} style={{ fill: "var(--background)" }} />
            ) : (
              <rect
                x={-framed}
                y={-framed}
                width={2 * framed}
                height={2 * framed}
                style={{ fill: "var(--background)" }}
              />
            )}
            <image
              href={photo.href}
              x={-half}
              y={-half}
              width={photoSize}
              height={photoSize}
              preserveAspectRatio="xMidYMid slice"
              clipPath={`url(#${clip})`}
            />
          </g>
        )
      })}
    </g>
  )
}

export function BlockedGeojsonNote({ host }: { host: string }) {
  return (
    <span className="pointer-events-auto">
      GeoJSON from {host} is not allowed on this server.{" "}
      <Link
        to="/settings"
        hash="outbound-access"
        className="rounded-sm font-medium text-foreground underline underline-offset-2 outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        Outbound access
      </Link>
    </span>
  )
}
