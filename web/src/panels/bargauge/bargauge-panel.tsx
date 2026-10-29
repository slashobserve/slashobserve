import { memo, useMemo } from "react"

import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"
import { cn } from "@/lib/utils"
import type { Reducer } from "@/spec"

import type { FieldResolver, LinkScope } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  barLinks,
  bargaugeModel,
  orientationOf,
  reducerOf,
  valueTextOf,
  type Bar,
  type Orientation,
} from "./bargauge-model"

type BarProps = {
  bar: Bar
  orientation: Orientation
  fields: FieldResolver
  reducer: Reducer
  scope: LinkScope
}

function Track({ bar, orientation }: Pick<BarProps, "bar" | "orientation">) {
  const vertical = orientation === "vertical"
  const percent = `${bar.fraction * 100}%`
  const now = Math.min(bar.max, Math.max(bar.min, bar.value ?? bar.min))
  return (
    <span
      role="meter"
      aria-label={bar.name}
      aria-valuenow={now}
      aria-valuemin={bar.min}
      aria-valuemax={bar.max}
      aria-valuetext={valueTextOf(bar)}
      data-threshold={bar.threshold}
      className={cn(
        "relative flex overflow-hidden rounded-[2px] bg-muted",
        vertical
          ? "mx-auto min-h-4 w-full max-w-12 min-w-3 flex-1 flex-col justify-end"
          : "h-2 w-full"
      )}
    >
      <span
        data-part="fill"
        className="block"
        style={{
          [vertical ? "height" : "width"]: percent,
          [vertical ? "width" : "height"]: "100%",
          backgroundColor: bar.color ?? "var(--series-1)",
        }}
      />
    </span>
  )
}

function Level({ bar, orientation }: Pick<BarProps, "bar" | "orientation">) {
  if (bar.threshold === "base" || bar.threshold === "ok") return null
  const Icon = thresholdIcons[bar.threshold]
  return (
    <span className="flex items-center gap-1 text-xs text-muted-foreground">
      <Icon
        className="size-3 shrink-0"
        strokeWidth={2.4}
        aria-hidden="true"
        style={{ color: bar.color ?? undefined }}
      />
      <span className={orientation === "vertical" ? "sr-only" : undefined}>
        {bar.word}
      </span>
    </span>
  )
}

function Value({ bar, orientation, fields, reducer, scope }: BarProps) {
  const [first] = barLinks(fields, bar, reducer, scope)
  const text = <span className="truncate">{bar.text}</span>
  const group = cn(
    "flex min-w-0 items-center tabular-nums",
    orientation === "vertical" ? "justify-center gap-1" : "shrink-0 gap-1.5"
  )
  return (
    <span className={group}>
      <Level bar={bar} orientation={orientation} />
      {first ? (
        <a
          {...anchorProps(first)}
          title={[first.title, valueTextOf(bar)].filter(Boolean).join(" - ")}
          className="flex min-w-0 items-center gap-1 underline decoration-border underline-offset-4 hover:decoration-current"
        >
          {text}
          {first.newTab ? <NewTabMark /> : null}
        </a>
      ) : (
        <span title={valueTextOf(bar)} className="flex min-w-0">
          {text}
        </span>
      )}
    </span>
  )
}

const HorizontalRow = memo(function HorizontalRow(props: BarProps) {
  const { bar } = props
  return (
    <div className="flex shrink-0 flex-col gap-[5px] text-[13px]">
      <div className="flex items-center justify-between gap-2">
        <span title={bar.name} className="min-w-0 truncate">
          {bar.name}
        </span>
        <Value {...props} />
      </div>
      <Track {...props} />
    </div>
  )
})

const VerticalColumn = memo(function VerticalColumn(props: BarProps) {
  const { bar } = props
  return (
    <div className="flex min-w-8 flex-1 basis-0 flex-col gap-1 text-[13px]">
      <Value {...props} />
      <Track {...props} />
      <span title={bar.name} className="truncate text-center">
        {bar.name}
      </span>
    </div>
  )
})

const BargaugePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  links,
  width,
  height,
}) => {
  const model = useMemo(
    () => bargaugeModel(frames, fields, display),
    [frames, fields, display]
  )
  const reducer = reducerOf(display)
  const orientation = orientationOf(display, { width, height })
  const vertical = orientation === "vertical"
  const Item = vertical ? VerticalColumn : HorizontalRow
  return (
    <div
      role="region"
      aria-label={`${panel.title} values`}
      tabIndex={0}
      data-orientation={orientation}
      className={cn(
        "flex px-3 pt-1.5 pb-3.5 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
        vertical
          ? "flex-row gap-2 overflow-x-auto"
          : "flex-col gap-3 overflow-y-auto"
      )}
      style={{ width, height }}
    >
      {model.bars.map((bar) => (
        <Item
          key={bar.key}
          bar={bar}
          orientation={orientation}
          fields={fields}
          reducer={reducer}
          scope={links}
        />
      ))}
    </div>
  )
}

export default BargaugePanel
