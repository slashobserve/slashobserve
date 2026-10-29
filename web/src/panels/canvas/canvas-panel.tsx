import { useMemo, type CSSProperties } from "react"
import { CircleIcon } from "lucide-react"

import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  canvasModel,
  type CanvasAlign,
  type CanvasElement,
  type CanvasLine,
  type MetricState,
} from "./canvas-model"

const justify: Record<CanvasAlign, string> = {
  left: "justify-start text-left",
  center: "justify-center text-center",
  right: "justify-end text-right",
}

const alignItems: Record<CanvasAlign, string> = {
  left: "items-start",
  center: "items-center",
  right: "items-end",
}

function percentBox(element: CanvasElement): CSSProperties {
  const { left, top, width, height } = element.box
  return {
    left: `${left}%`,
    top: `${top}%`,
    width: `${width}%`,
    height: `${height}%`,
  }
}

function Shape({ element }: { element: CanvasElement }) {
  const { left, top, width, height } = element.box
  const style = {
    fill: element.fill ?? "none",
    stroke: element.stroke ?? "var(--border)",
    strokeWidth: 1,
  }
  return element.kind === "ellipse" ? (
    <ellipse
      cx={left + width / 2}
      cy={top + height / 2}
      rx={width / 2}
      ry={height / 2}
      vectorEffect="non-scaling-stroke"
      style={style}
    />
  ) : (
    <rect
      x={left}
      y={top}
      width={width}
      height={height}
      vectorEffect="non-scaling-stroke"
      style={style}
    />
  )
}

function Line({ line }: { line: CanvasLine }) {
  return (
    <line
      x1={line.x1}
      y1={line.y1}
      x2={line.x2}
      y2={line.y2}
      vectorEffect="non-scaling-stroke"
      style={{ stroke: line.color, strokeWidth: line.width }}
    />
  )
}

function StateMark({ state }: { state: MetricState }) {
  const Icon = state.level === "base" ? CircleIcon : thresholdIcons[state.level]
  return (
    <span
      data-canvas-state={state.level}
      className="flex shrink-0 items-center gap-1 text-[0.8em]"
      style={{ color: state.labelColor ?? undefined }}
    >
      <Icon
        className="size-[1em]"
        strokeWidth={2.4}
        fill={state.level === "base" ? "currentColor" : "none"}
        aria-hidden="true"
        style={{ color: state.color }}
      />
      {state.label}
    </span>
  )
}

function Content({ element }: { element: CanvasElement }) {
  if (element.kind !== "metric") {
    return element.text === null ? null : (
      <span title={element.text} className="min-w-0 truncate">
        {element.text}
      </span>
    )
  }
  return (
    <div className={cn("flex min-w-0 flex-col", alignItems[element.align])}>
      {element.text === null ? null : (
        <span
          title={element.text}
          className="max-w-full truncate text-[0.8em] opacity-70"
        >
          {element.text}
        </span>
      )}
      <div className="flex max-w-full min-w-0 items-baseline gap-1.5">
        <span
          title={element.valueText ?? undefined}
          data-canvas-value=""
          className="min-w-0 truncate font-semibold tabular-nums"
        >
          {element.valueText}
        </span>
        {element.state ? <StateMark state={element.state} /> : null}
      </div>
    </div>
  )
}

function Box({ element }: { element: CanvasElement }) {
  const boxed = element.kind === "text" || element.kind === "metric"
  return (
    <div
      data-canvas-element={element.kind}
      data-element-id={element.id}
      className={cn(
        "absolute flex min-w-0 items-center overflow-hidden px-1 leading-tight",
        justify[element.align],
        boxed && element.fill && "rounded-md"
      )}
      style={{
        ...percentBox(element),
        fontSize: element.fontSize,
        color: element.textColor ?? undefined,
        backgroundColor: boxed ? (element.fill ?? undefined) : undefined,
      }}
    >
      <Content element={element} />
    </div>
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
  const model = useMemo(
    () => canvasModel(frames, fields, display),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- text on a fill reads the live theme tokens, so a theme flip must recompute it
    [frames, fields, display, theme]
  )
  const shapes = model.elements.filter(
    (element) => element.kind === "rectangle" || element.kind === "ellipse"
  )
  const labelled = model.elements.filter(
    (element) =>
      element.kind === "text" ||
      element.kind === "metric" ||
      element.text !== null
  )
  return (
    <div
      role="group"
      aria-label={panel.title}
      data-elements={model.elements.length}
      className="relative min-h-0 min-w-0 overflow-hidden text-foreground"
      style={{ width, height }}
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
        className="absolute inset-0 size-full"
      >
        {shapes.map((element) => (
          <Shape key={element.key} element={element} />
        ))}
        {model.lines.map((line) => (
          <Line key={line.key} line={line} />
        ))}
      </svg>
      {labelled.map((element) => (
        <Box key={element.key} element={element} />
      ))}
    </div>
  )
}

export default CanvasPanel
