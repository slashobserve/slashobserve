import { memo, useMemo, useState } from "react"

import { cn } from "@/lib/utils"

import type { PanelRenderer } from "../types"
import {
  donutRatio,
  legendOf,
  percentText,
  pieModel,
  pieTypeOf,
  sliceLabel,
  slicePath,
  summaryLabel,
  type LeftOut,
  type PieLegend,
  type Slice,
} from "./pie-model"

const center = 50
const radius = 48

type LegendEntry = {
  key: string
  name: string
  text: string
  color: string
  percent: string
}

type Hover = { hovered: string | null; onHover: (key: string | null) => void }

function legendEntries(slices: Slice[], left: LeftOut[]): LegendEntry[] {
  return [
    ...slices.map((slice) => ({
      key: slice.key,
      name: slice.name,
      text: slice.text,
      color: slice.color,
      percent: percentText(slice.percent),
    })),
    ...left.map((item) => ({
      key: item.key,
      name: item.name,
      text: item.text,
      color: item.color,
      percent: item.value === null ? "" : percentText(0),
    })),
  ]
}

const Legend = memo(function Legend({
  entries,
  placement,
  onHover,
}: {
  entries: LegendEntry[]
  placement: Exclude<PieLegend, "hidden">
} & Pick<Hover, "onHover">) {
  const bottom = placement === "bottom"
  return (
    <ul
      aria-label="Legend"
      data-placement={placement}
      className={cn(
        "m-0 flex list-none overflow-y-auto p-0 text-xs",
        bottom
          ? "max-h-[40%] shrink-0 flex-wrap gap-x-4 gap-y-1 px-3 pb-2"
          : "w-[clamp(140px,35%,260px)] shrink-0 flex-col gap-1 py-2 pr-3"
      )}
    >
      {entries.map((entry) => (
        <li
          key={entry.key}
          data-key={entry.key}
          onMouseEnter={() => onHover(entry.key)}
          onMouseLeave={() => onHover(null)}
          className={cn(
            "min-w-0",
            bottom
              ? "flex items-center gap-1.5"
              : "grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-1.5"
          )}
        >
          <span
            aria-hidden="true"
            className="size-2.5 shrink-0 self-center"
            style={{ backgroundColor: entry.color }}
          />
          <span title={entry.name} className="min-w-0 truncate">
            {entry.name}
          </span>
          <span
            className={cn(
              "text-muted-foreground tabular-nums",
              bottom ? "shrink-0" : "col-start-2 row-start-2"
            )}
          >
            {entry.text}
          </span>
          <span
            className={cn(
              "shrink-0 text-muted-foreground tabular-nums",
              !bottom && "col-start-3 row-start-1"
            )}
          >
            {entry.percent}
          </span>
        </li>
      ))}
    </ul>
  )
})

function Pie({
  slices,
  donut,
  hovered,
  onHover,
}: { slices: Slice[]; donut: boolean } & Hover) {
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
          onMouseLeave={() => onHover(null)}
          style={{
            fill: slice.color,
            stroke: "var(--background)",
            strokeWidth: 1,
            opacity: hovered !== null && hovered !== slice.key ? 0.4 : 1,
          }}
        >
          <title>{sliceLabel(slice)}</title>
        </path>
      ))}
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

const PiechartPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  width,
  height,
}) => {
  const model = useMemo(
    () => pieModel(frames, fields, display),
    [frames, fields, display]
  )
  const [hovered, setHovered] = useState<string | null>(null)
  const legend = legendOf(display)
  const pieType = pieTypeOf(display)
  const entries = useMemo(
    () => legendEntries(model.slices, model.left),
    [model.slices, model.left]
  )
  return (
    <div
      data-slices={model.slices.length}
      data-pie-type={pieType}
      data-legend={legend}
      className={cn(
        "flex min-h-0 min-w-0",
        legend === "bottom" ? "flex-col" : "flex-row"
      )}
      style={{ width, height }}
    >
      <div className="min-h-0 min-w-0 flex-1 p-2">
        <Pie
          slices={model.slices}
          donut={pieType === "donut"}
          hovered={hovered}
          onHover={setHovered}
        />
      </div>
      {legend === "hidden" ? null : (
        <Legend entries={entries} placement={legend} onHover={setHovered} />
      )}
    </div>
  )
}

export default PiechartPanel
