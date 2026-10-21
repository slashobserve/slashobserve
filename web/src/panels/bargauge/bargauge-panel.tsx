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
  reducerOf,
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
  const valueText = bar.word ? `${bar.text}, ${bar.word}` : bar.text
  return (
    <span
      role="meter"
      aria-label={bar.name}
      aria-valuenow={bar.value ?? undefined}
      aria-valuemin={bar.min}
      aria-valuemax={bar.max}
      aria-valuetext={valueText}
      data-threshold={bar.threshold}
      className={cn(
        "relative flex overflow-hidden rounded-[2px] border border-border",
        vertical
          ? "mx-auto min-h-4 w-full max-w-12 min-w-3 flex-1 flex-col justify-end"
          : "h-2"
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

function Value({ bar, orientation, fields, reducer, scope }: BarProps) {
  const [first] = barLinks(fields, bar, reducer, scope)
  const Icon = bar.threshold === "base" ? null : thresholdIcons[bar.threshold]
  const content = (
    <>
      {Icon ? (
        <Icon
          className="size-3.5 shrink-0"
          aria-hidden="true"
          style={{ color: bar.color ?? undefined }}
        />
      ) : null}
      <span className="truncate">{bar.text}</span>
      {bar.word ? <span className="sr-only">, {bar.word}</span> : null}
    </>
  )
  const className = cn(
    "flex min-w-0 items-center gap-1 tabular-nums",
    orientation === "vertical" ? "justify-center" : "justify-end"
  )
  return first ? (
    <a
      {...anchorProps(first)}
      title={[first.title, bar.word].filter(Boolean).join(" - ") || undefined}
      className={cn(
        className,
        "underline decoration-border underline-offset-4 hover:decoration-current"
      )}
    >
      {content}
      {first.newTab ? <NewTabMark /> : null}
    </a>
  ) : (
    <span title={bar.word || undefined} className={className}>
      {content}
    </span>
  )
}

const HorizontalRow = memo(function HorizontalRow(props: BarProps) {
  const { bar } = props
  return (
    <div className="grid h-7 shrink-0 grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-x-3 text-[13px]">
      <span title={bar.name} className="truncate">
        {bar.name}
      </span>
      <Track {...props} />
      <Value {...props} />
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

export const BargaugePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  links,
  width,
  height,
}) => {
  const model = useMemo(
    () => bargaugeModel(frames, fields, display, { width, height }),
    [frames, fields, display, width, height]
  )
  const reducer = reducerOf(display)
  const vertical = model.orientation === "vertical"
  const Item = vertical ? VerticalColumn : HorizontalRow
  return (
    <div
      role="region"
      aria-label={`${panel.title} values`}
      tabIndex={0}
      data-orientation={model.orientation}
      className={cn(
        "flex px-3 pt-1 pb-2 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
        vertical ? "flex-row gap-2 overflow-x-auto" : "flex-col overflow-y-auto"
      )}
      style={{ width, height }}
    >
      {model.bars.map((bar) => (
        <Item
          key={bar.key}
          bar={bar}
          orientation={model.orientation}
          fields={fields}
          reducer={reducer}
          scope={links}
        />
      ))}
    </div>
  )
}

export default BargaugePanel
