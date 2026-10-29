import {
  memo,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react"
import { MaximizeIcon, ZoomInIcon, ZoomOutIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

import { slicePath } from "../piechart/pie-model"
import type { PanelRenderer } from "../types"
import {
  edgeRows,
  graphModel,
  nodeRows,
  type GraphArc,
  type GraphDetail,
  type GraphEdge,
  type GraphNode,
} from "./graph-model"
import { gridLayout, layeredLayout, type Layout, type Point } from "./layout"

const cellWidth = 200
const cellHeight = 120
const nodeRadius = 26
const arcWidth = 4
const ringOuter = nodeRadius + 2 + arcWidth
const edgeInset = ringOuter + 2
const labelDrop = ringOuter + 14
const sideRoom = 90
const bottomRoom = labelDrop + 20
const topRoom = ringOuter + 4
const maxFit = 1.5
const minZoom = 0.25
const maxZoom = 4
const zoomStep = 1.25
const dragThreshold = 3
const parallelShift = 5
const fullTurn = Math.PI * 2

const mutedStroke = "var(--muted-foreground)"
const strongStroke = "var(--foreground)"

type View = { zoom: number; panX: number; panY: number }
type Selection = { kind: "node" | "edge"; id: string }
type Drag = {
  x: number
  y: number
  panX: number
  panY: number
  moved: boolean
}

const initialView: View = { zoom: 1, panX: 0, panY: 0 }

function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text
}

function layoutOf(
  nodes: GraphNode[],
  edges: GraphEdge[],
  display: Record<string, unknown>
): Layout {
  return display.layout === "grid"
    ? gridLayout(nodes)
    : layeredLayout(nodes, edges)
}

function scaled(layout: Layout): Map<string, Point> {
  return new Map(
    [...layout].map(([id, { x, y }]) => [
      id,
      { x: x * cellWidth, y: y * cellHeight },
    ])
  )
}

function boundsOf(points: Map<string, Point>) {
  const all = [...points.values()]
  const xs = all.map((point) => point.x)
  const ys = all.map((point) => point.y)
  const left = Math.min(...xs) - sideRoom
  const right = Math.max(...xs) + sideRoom
  const top = Math.min(...ys) - topRoom
  const bottom = Math.max(...ys) + bottomRoom
  return {
    centerX: (left + right) / 2,
    centerY: (top + bottom) / 2,
    width: right - left,
    height: bottom - top,
  }
}

function arcSpans(arcs: GraphArc[]) {
  let start = 0
  return arcs.map((arc) => {
    const span = { arc, start, end: start + arc.fraction * fullTurn }
    start = span.end
    return span
  })
}

function activate(event: KeyboardEvent, action: () => void) {
  if (event.key !== "Enter" && event.key !== " ") return
  event.preventDefault()
  action()
}

const focusClass =
  "cursor-pointer outline-none focus-visible:outline-2 focus-visible:outline-ring"

const haloStyle = {
  paintOrder: "stroke",
  stroke: "var(--card)",
  strokeWidth: 3,
  strokeLinejoin: "round",
} as const

type NodeProps = {
  node: GraphNode
  at: Point
  selected: boolean
  onHover: (id: string | null) => void
  onSelect: (selection: Selection) => void
  suppressed: RefObject<boolean>
}

const NodeView = memo(function NodeView({
  node,
  at,
  selected,
  onHover,
  onSelect,
  suppressed,
}: NodeProps) {
  const choose = () => {
    if (!suppressed.current) onSelect({ kind: "node", id: node.id })
  }
  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={`Service ${node.title}`}
      data-node={node.id}
      transform={`translate(${at.x} ${at.y})`}
      className={focusClass}
      onPointerEnter={() => onHover(node.id)}
      onPointerLeave={() => onHover(null)}
      onFocus={() => onHover(node.id)}
      onBlur={() => onHover(null)}
      onClick={choose}
      onKeyDown={(event) => activate(event, choose)}
    >
      <title>
        {node.subtitle ? `${node.title} (${node.subtitle})` : node.title}
      </title>
      <circle
        r={nodeRadius}
        style={{
          fill: "var(--card)",
          stroke: selected ? strongStroke : (node.color ?? "var(--border)"),
          strokeWidth: selected ? 2 : 1,
        }}
      />
      {arcSpans(node.arcs).map(({ arc, start, end }, index) => (
        <path
          key={`${arc.name}#${index}`}
          d={slicePath(0, 0, ringOuter, ringOuter - arcWidth, start, end)}
          fillRule="evenodd"
          style={{ fill: arc.color }}
        />
      ))}
      {node.mainStat ? (
        <text
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={11}
          style={{ fill: strongStroke }}
        >
          {clip(node.mainStat.display.text, 7)}
        </text>
      ) : null}
      <text
        y={labelDrop}
        textAnchor="middle"
        fontSize={12}
        style={{ ...haloStyle, fill: strongStroke }}
      >
        {clip(node.title, 22)}
      </text>
      {node.subtitle ? (
        <text
          y={labelDrop + 14}
          textAnchor="middle"
          fontSize={12}
          style={{ ...haloStyle, fill: mutedStroke }}
        >
          {clip(node.subtitle, 22)}
        </text>
      ) : null}
    </g>
  )
})

type EdgeProps = {
  edge: GraphEdge
  from: Point
  to: Point
  shifted: boolean
  stroke: string
  markerId: string
  dimmed: boolean
  onSelect: (selection: Selection) => void
  suppressed: RefObject<boolean>
}

function edgeEnds(from: Point, to: Point, shifted: boolean) {
  const dx = to.x - from.x
  const dy = to.y - from.y
  const length = Math.hypot(dx, dy) || 1
  const ux = dx / length
  const uy = dy / length
  const shift = shifted ? parallelShift : 0
  const ox = -uy * shift
  const oy = ux * shift
  return {
    x1: from.x + ox + ux * edgeInset,
    y1: from.y + oy + uy * edgeInset,
    x2: to.x + ox - ux * edgeInset,
    y2: to.y + oy - uy * edgeInset,
  }
}

const EdgeView = memo(function EdgeView({
  edge,
  from,
  to,
  shifted,
  stroke,
  markerId,
  dimmed,
  onSelect,
  suppressed,
}: EdgeProps) {
  const { x1, y1, x2, y2 } = edgeEnds(from, to, shifted)
  const choose = () => {
    if (!suppressed.current) onSelect({ kind: "edge", id: edge.id })
  }
  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={`Connection ${edge.source} to ${edge.target}`}
      data-edge={edge.id}
      opacity={dimmed ? 0.25 : 1}
      className={focusClass}
      onClick={choose}
      onKeyDown={(event) => activate(event, choose)}
    >
      <line
        x1={x1}
        y1={y1}
        x2={x2}
        y2={y2}
        markerEnd={`url(#${markerId})`}
        style={{ stroke, strokeWidth: edge.thickness }}
      />
      <line
        x1={x1}
        y1={y1}
        x2={x2}
        y2={y2}
        style={{
          stroke: "transparent",
          strokeWidth: Math.max(edge.thickness, 12),
        }}
      />
      {edge.mainStat ? (
        <text
          x={(x1 + x2) / 2}
          y={(y1 + y2) / 2}
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={11}
          style={{ ...haloStyle, fill: mutedStroke }}
        >
          {edge.mainStat.display.text}
        </text>
      ) : null}
    </g>
  )
})

function Details({
  title,
  subtitle,
  rows,
  panelRef,
}: {
  title: string
  subtitle: string
  rows: GraphDetail[]
  panelRef: RefObject<HTMLDivElement | null>
}) {
  return (
    <div
      ref={panelRef}
      role="region"
      aria-label="Details"
      className="absolute top-2 right-2 z-10 max-h-[calc(100%-1rem)] max-w-[60%] min-w-40 overflow-auto rounded-control border border-border bg-popover px-2.5 py-2 text-xs text-popover-foreground"
    >
      <div className="font-medium break-words">{title}</div>
      {subtitle ? (
        <div className="break-words text-muted-foreground">{subtitle}</div>
      ) : null}
      {rows.length > 0 ? (
        <dl className="mt-1.5 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5">
          {rows.map((row, index) => (
            <div key={`${row.name}#${index}`} className="contents">
              <dt className="text-muted-foreground">{row.name}</dt>
              <dd className="m-0 break-words tabular-nums">{row.text}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </div>
  )
}

function useDismiss(
  open: boolean,
  inside: RefObject<HTMLElement | null>,
  close: () => void
) {
  useEffect(() => {
    if (!open) return
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") close()
    }
    const onPointer = (event: globalThis.PointerEvent) => {
      if (!inside.current?.contains(event.target as Node)) close()
    }
    document.addEventListener("keydown", onKey)
    document.addEventListener("pointerdown", onPointer)
    return () => {
      document.removeEventListener("keydown", onKey)
      document.removeEventListener("pointerdown", onPointer)
    }
  }, [open, inside, close])
}

type GraphProps = {
  nodes: GraphNode[]
  edges: GraphEdge[]
  note: string | null
  display: Record<string, unknown>
  width: number
  height: number
}

function Graph({ nodes, edges, note, display, width, height }: GraphProps) {
  const markerPrefix = useId().replaceAll(":", "")
  const points = useMemo(
    () => scaled(layoutOf(nodes, edges, display)),
    [nodes, edges, display]
  )
  const bounds = useMemo(() => boundsOf(points), [points])
  const nodesById = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes]
  )
  const pairs = useMemo(
    () => new Set(edges.map((edge) => `${edge.source}\n${edge.target}`)),
    [edges]
  )
  const [view, setView] = useState(initialView)
  const [hovered, setHovered] = useState<string | null>(null)
  const [selection, setSelection] = useState<Selection | null>(null)
  const drag = useRef<Drag | null>(null)
  const suppressed = useRef(false)
  const detailsRef = useRef<HTMLDivElement>(null)
  const close = useCallback(() => setSelection(null), [])
  useDismiss(selection !== null, detailsRef, close)

  const fit = Math.min(
    maxFit,
    Math.max(1, width) / bounds.width,
    Math.max(1, height) / bounds.height
  )
  const scale = fit * view.zoom
  const transform = `translate(${width / 2 + view.panX} ${height / 2 + view.panY}) scale(${scale}) translate(${-bounds.centerX} ${-bounds.centerY})`

  const strokes = edges.map((edge) => {
    const lit =
      hovered !== null && (edge.source === hovered || edge.target === hovered)
    const chosen = selection?.kind === "edge" && selection.id === edge.id
    return lit || chosen ? strongStroke : (edge.color ?? mutedStroke)
  })
  const markerColors = [...new Set(strokes)]

  function zoomBy(factor: number) {
    setView((current) => {
      const zoom = Math.min(maxZoom, Math.max(minZoom, current.zoom * factor))
      const ratio = zoom / current.zoom
      return {
        zoom,
        panX: current.panX * ratio,
        panY: current.panY * ratio,
      }
    })
  }

  function onPointerDown(event: PointerEvent<SVGSVGElement>) {
    if (event.button !== 0) return
    suppressed.current = false
    drag.current = {
      x: event.clientX,
      y: event.clientY,
      panX: view.panX,
      panY: view.panY,
      moved: false,
    }
  }

  function onPointerMove(event: PointerEvent<SVGSVGElement>) {
    const start = drag.current
    if (!start) return
    const dx = event.clientX - start.x
    const dy = event.clientY - start.y
    if (!start.moved) {
      if (Math.hypot(dx, dy) < dragThreshold) return
      start.moved = true
      suppressed.current = true
      event.currentTarget.setPointerCapture(event.pointerId)
    }
    setView((current) => ({
      ...current,
      panX: start.panX + dx,
      panY: start.panY + dy,
    }))
  }

  function endDrag() {
    drag.current = null
  }

  const selectedNode =
    selection?.kind === "node" ? nodesById.get(selection.id) : undefined
  const selectedEdge =
    selection?.kind === "edge"
      ? edges.find((edge) => edge.id === selection.id)
      : undefined
  const titleOf = (id: string) => nodesById.get(id)?.title ?? id

  return (
    <div
      className="relative overflow-hidden"
      style={{ width, height }}
      data-nodes={nodes.length}
      data-edges={edges.length}
    >
      <svg
        role="group"
        aria-label={`Node graph, ${nodes.length} services, ${edges.length} connections`}
        width={width}
        height={height}
        className="block cursor-grab active:cursor-grabbing"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        <defs>
          {markerColors.map((color, index) => (
            <marker
              key={color}
              id={`${markerPrefix}-${index}`}
              viewBox="0 0 10 10"
              refX={10}
              refY={5}
              markerWidth={10}
              markerHeight={10}
              markerUnits="userSpaceOnUse"
              orient="auto"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" style={{ fill: color }} />
            </marker>
          ))}
        </defs>
        <g transform={transform}>
          {edges.map((edge, index) => {
            const from = points.get(edge.source)
            const to = points.get(edge.target)
            if (!from || !to) return null
            const lit = hovered === null || strokes[index] === strongStroke
            return (
              <EdgeView
                key={edge.id}
                edge={edge}
                from={from}
                to={to}
                shifted={pairs.has(`${edge.target}\n${edge.source}`)}
                stroke={strokes[index]}
                markerId={`${markerPrefix}-${markerColors.indexOf(strokes[index])}`}
                dimmed={!lit}
                onSelect={setSelection}
                suppressed={suppressed}
              />
            )
          })}
          {nodes.map((node) => {
            const at = points.get(node.id)
            if (!at) return null
            return (
              <NodeView
                key={node.id}
                node={node}
                at={at}
                selected={
                  selection?.kind === "node" && selection.id === node.id
                }
                onHover={setHovered}
                onSelect={setSelection}
                suppressed={suppressed}
              />
            )
          })}
        </g>
      </svg>
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
      {note ? (
        <p className="absolute bottom-2 left-2 m-0 text-xs text-muted-foreground">
          {note}
        </p>
      ) : null}
      {selectedNode ? (
        <Details
          panelRef={detailsRef}
          title={selectedNode.title}
          subtitle={selectedNode.subtitle}
          rows={nodeRows(selectedNode)}
        />
      ) : null}
      {selectedEdge ? (
        <Details
          panelRef={detailsRef}
          title={`${titleOf(selectedEdge.source)} to ${titleOf(selectedEdge.target)}`}
          subtitle=""
          rows={edgeRows(selectedEdge)}
        />
      ) : null}
    </div>
  )
}

const NodeGraphPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const model = useMemo(() => graphModel(frames, fields), [frames, fields])
  if (model.reason !== null) {
    return (
      <div
        className="flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
        style={{ width, height }}
      >
        {model.reason}
      </div>
    )
  }
  if (model.nodes.length === 0) {
    return (
      <div
        className="flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
        style={{ width, height }}
      >
        No services to show.
      </div>
    )
  }
  return (
    <Graph
      nodes={model.nodes}
      edges={model.edges}
      note={model.note}
      display={display}
      width={width}
      height={height}
    />
  )
}

export default NodeGraphPanel
