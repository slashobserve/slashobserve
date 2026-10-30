import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react"

import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

import { resolveColor } from "../colors"
import { cssColorToRgb, darkText, lightText, readableText } from "../contrast"
import { Legend, rootFontSize, type LegendItem } from "../legend/legend"
import { legendOrder } from "../legend/legend-model"
import { LegendTable, useLegendTextMeasure } from "../legend/legend-table"
import { legendTableWidth, rightLegendWidth } from "../legend/legend-width"
import {
  emptySeriesState,
  useSeriesStore,
  visibleSeries,
} from "../timeseries/series-state"
import type { PanelRenderer } from "../types"
import {
  donutRatio,
  labelPlacement,
  lineOffsets,
  legendCells,
  pieEntries,
  pieModel,
  pieOptions,
  sliceLabelLines,
  slicePath,
  summaryLabel,
  tooltipRows,
  type LegendColumn,
  type PieOptions,
  type Slice,
  type TooltipRow,
} from "./pie-model"

const center = 50
const radius = 48
const legendShare = 0.4
const tooltipOffset = 12

const columnHeaders: Record<LegendColumn, string> = {
  value: "Value",
  percent: "Percent",
}

type Box = { width: number; height: number }
type Pointer = { x: number; y: number }

function labelTextColors(
  slices: Slice[],
  theme: string
): ReadonlyMap<string, string> {
  const unparsed = theme === "dark" ? lightText : darkText
  return new Map(
    slices.map((slice) => {
      const rgb = cssColorToRgb(resolveColor(slice.color))
      return [slice.key, rgb ? readableText(rgb) : unparsed]
    })
  )
}

function useBox() {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  const [box, setBox] = useState<Box | null>(null)
  useLayoutEffect(() => {
    if (!node) return
    const observer = new ResizeObserver(([entry]) =>
      setBox({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      })
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [node])
  return { node, box, ref: setNode }
}

function SliceLabels({
  slices,
  options,
  textColors,
  side,
  donut,
}: {
  slices: Slice[]
  options: PieOptions
  textColors: ReadonlyMap<string, string>
  side: number
  donut: boolean
}) {
  const unit = 100 / side
  const radiusPx = radius / unit
  const innerPx = donut ? radiusPx * donutRatio : 0
  const sizes = { title: options.titleSize, value: options.valueSize }
  return (
    <g aria-hidden="true" pointerEvents="none" data-part="slice-labels">
      {slices.map((slice) => {
        const lines = sliceLabelLines(slice, options.sliceLabels, sizes)
        const placed = labelPlacement(slice, lines, radiusPx, innerPx)
        if (!placed.fits) return null
        const offsets = lineOffsets(lines)
        return (
          <g key={slice.key} data-slice={slice.key}>
            {lines.map((line, index) => (
              <text
                key={index}
                x={center + placed.x * unit}
                y={center + (placed.y + offsets[index]) * unit}
                fontSize={line.size * unit}
                textAnchor="middle"
                dominantBaseline="central"
                style={{ fill: textColors.get(slice.key) }}
              >
                {line.text}
              </text>
            ))}
          </g>
        )
      })}
    </g>
  )
}

function Pie({
  slices,
  donut,
  hovered,
  onHover,
  onMove,
  labels,
}: {
  slices: Slice[]
  donut: boolean
  hovered: string | null
  onHover: (key: string | null) => void
  onMove: (event: MouseEvent) => void
  labels: ReactNode
}) {
  const inner = donut ? radius * donutRatio : 0
  const outlined = slices.find((slice) => slice.key === hovered)
  return (
    <svg
      role="img"
      aria-label={summaryLabel(slices)}
      viewBox="0 0 100 100"
      className="size-full"
    >
      {slices.length === 0 ? (
        <circle
          cx={center}
          cy={center}
          r={radius}
          style={{ fill: "none", stroke: "var(--border)" }}
        />
      ) : null}
      {slices.map((slice) => (
        <path
          key={slice.key}
          data-slice={slice.key}
          d={slicePath(
            center,
            center,
            radius,
            inner,
            slice.startAngle,
            slice.endAngle
          )}
          fillRule="evenodd"
          vectorEffect="non-scaling-stroke"
          onMouseEnter={() => onHover(slice.key)}
          onMouseMove={onMove}
          onMouseLeave={() => onHover(null)}
          style={{
            fill: slice.color,
            stroke: "var(--background)",
            strokeWidth: 1,
            opacity: hovered !== null && hovered !== slice.key ? 0.4 : 1,
          }}
        />
      ))}
      {labels}
      {outlined ? (
        <path
          data-part="outline"
          d={slicePath(
            center,
            center,
            radius,
            inner,
            outlined.startAngle,
            outlined.endAngle
          )}
          fillRule="evenodd"
          vectorEffect="non-scaling-stroke"
          pointerEvents="none"
          style={{
            fill: "none",
            stroke: "var(--foreground)",
            strokeWidth: 1.5,
          }}
        />
      ) : null}
    </svg>
  )
}

function Tooltip({
  rows,
  pointer,
  box,
  maxWidth,
  maxHeight,
}: {
  rows: TooltipRow[]
  pointer: Pointer
  box: Box
  maxWidth?: number
  maxHeight?: number
}) {
  const left = pointer.x > box.width / 2
  const above = pointer.y > box.height / 2
  const shift = `calc(-100% - ${tooltipOffset}px)`
  return (
    <div
      role="tooltip"
      data-part="tooltip"
      className="pointer-events-none absolute z-10 min-w-32 overflow-hidden rounded-md border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        left: pointer.x,
        top: pointer.y,
        maxWidth: maxWidth ?? "min(20rem, 90%)",
        maxHeight,
        transform: `translate(${left ? shift : `${tooltipOffset}px`}, ${above ? shift : `${tooltipOffset}px`})`,
      }}
    >
      {rows.map((row) => (
        <div
          key={row.key}
          data-active={row.active}
          className={cn("flex items-center gap-2", row.active && "font-medium")}
        >
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0"
            style={{ backgroundColor: row.color }}
          />
          <span className="min-w-0 flex-1 truncate">{row.name}</span>
          <span className="shrink-0 tabular-nums">{row.text}</span>
          <span className="shrink-0 text-muted-foreground tabular-nums">
            {row.percent}
          </span>
        </div>
      ))}
    </div>
  )
}

const PiechartPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  width,
  height,
  stateScope,
}) => {
  const { resolved: theme } = useTheme()
  const options = useMemo(() => pieOptions(display), [display])
  const entries = useMemo(
    () => pieEntries(frames, fields, display),
    [frames, fields, display]
  )
  const keys = useMemo(() => entries.map(({ key }) => key), [entries])
  const stateKey = `${stateScope}:${panel.id}`
  const stored = useSeriesStore((store) => store.panels[stateKey])
  const commitKeys = useSeriesStore((store) => store.commit)
  const toggleKey = useSeriesStore((store) => store.toggle)
  const storedHidden = (stored ?? emptySeriesState).hidden

  const order = useMemo(
    () =>
      legendOrder(
        entries.map((entry) => ({
          key: entry.key,
          label: entry.name,
          values: [entry.value],
          hidden: entry.hideInLegend,
        })),
        {
          mode: options.legendMode,
          values: [],
          sortBy: options.legendSortBy === "value" ? "lastNotNull" : "name",
          sortDesc: options.legendSortDesc,
          hideZero: false,
          hideEmpty: false,
        }
      ),
    [entries, options]
  )
  const listed = useMemo(
    () => order.map((index) => entries[index]),
    [order, entries]
  )
  const legendShown = options.legend !== "hidden" && listed.length > 0
  const hidden = useMemo(
    () => visibleSeries(storedHidden, keys, legendShown),
    [storedHidden, keys, legendShown]
  )
  const model = useMemo(() => pieModel(entries, hidden), [entries, hidden])

  useLayoutEffect(() => {
    commitKeys(stateKey, keys)
  }, [commitKeys, stateKey, keys])

  const toggle = useCallback(
    (key: string) => toggleKey(stateKey, key),
    [toggleKey, stateKey]
  )

  const [hovered, setHovered] = useState<string | null>(null)
  const [pointer, setPointer] = useState<Pointer | null>(null)
  const { node, box, ref: boxRef } = useBox()
  const hover = useCallback((key: string | null) => {
    setHovered(key)
    if (key === null) setPointer(null)
  }, [])
  const move = useCallback(
    (event: MouseEvent) => {
      if (!node) return
      const rect = node.getBoundingClientRect()
      setPointer({ x: event.clientX - rect.left, y: event.clientY - rect.top })
    },
    [node]
  )

  const donut = options.pieType === "donut"
  const side = box ? Math.min(box.width, box.height) : 0
  const textColors = useMemo(
    () => labelTextColors(model.slices, theme),
    [model.slices, theme]
  )

  const bottom = options.legend === "bottom"
  const table = options.legendMode === "table"
  const columns = options.legendColumns
  const headers = useMemo(
    () => columns.map((column) => (table ? columnHeaders[column] : "")),
    [columns, table]
  )
  const cells = useMemo(
    () => legendCells(listed, model.percents, columns),
    [listed, model.percents, columns]
  )
  const textMeasure = useLegendTextMeasure()
  const rightLegend = legendShown && !bottom
  const tableWidth = useMemo(() => {
    if (!rightLegend || !table || columns.length === 0) return null
    const rem = rootFontSize()
    return legendTableWidth(
      listed.map(({ name }) => name),
      headers,
      cells,
      textMeasure(rem),
      rem
    )
  }, [rightLegend, table, listed, columns, headers, cells, textMeasure])
  const legendWidth = rightLegend
    ? rightLegendWidth(tableWidth, width, options.legendWidth)
    : 0

  const items: LegendItem[] = listed.map((entry) => ({
    key: entry.key,
    label: entry.name,
    color: entry.color,
    line: "solid",
    style: "bars",
  }))
  const legendProps = {
    items,
    reducers: columns,
    headers,
    cells,
    hidden,
    placement: bottom ? "bottom" : "right",
    available: height * legendShare,
    width: legendWidth,
    onToggle: toggle,
    onHover: hover,
  } as const

  const rows = useMemo(
    () => tooltipRows(entries, model.percents, hovered, options, hidden),
    [entries, model.percents, hovered, options, hidden]
  )
  const labelsShown = options.sliceLabels.length > 0 && side > 0

  return (
    <div
      data-slices={model.slices.length}
      data-pie-type={options.pieType}
      data-legend={options.legend}
      data-legend-mode={options.legendMode}
      data-tooltip={options.tooltip}
      className={cn("flex min-h-0 min-w-0", bottom ? "flex-col" : "flex-row")}
      style={{ width, height }}
    >
      <div ref={boxRef} className="relative min-h-0 min-w-0 flex-1 p-2">
        <Pie
          slices={model.slices}
          donut={donut}
          hovered={hovered}
          onHover={hover}
          onMove={move}
          labels={
            labelsShown ? (
              <SliceLabels
                slices={model.slices}
                options={options}
                textColors={textColors}
                side={side}
                donut={donut}
              />
            ) : null
          }
        />
        {pointer && box && rows.length > 0 ? (
          <Tooltip
            rows={rows}
            pointer={pointer}
            box={box}
            maxWidth={options.tooltipMaxWidth}
            maxHeight={options.tooltipMaxHeight}
          />
        ) : null}
      </div>
      {legendShown && table ? (
        <LegendTable
          {...legendProps}
          sortBy={options.legendSortBy}
          sortDesc={options.legendSortDesc}
        />
      ) : null}
      {legendShown && !table ? <Legend {...legendProps} /> : null}
    </div>
  )
}

export default PiechartPanel
