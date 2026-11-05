import { memo, useMemo, type CSSProperties } from "react"
import { ZapIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  anchorProps,
  NewTabMark,
  oneClickProps,
} from "@/dashboards/dashboard-links"
import { cn } from "@/lib/utils"
import type { Reducer } from "@/spec"

import { ActionMenuItems } from "../actions/action-menu-items"
import {
  runFieldAction,
  useStableActionRunner,
  type StableActionRunner,
} from "../actions/action-runner"
import { oneClickTarget, type FieldAction } from "../actions/field-actions"
import type { FieldResolver, LinkScope } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  barActions,
  barLimits,
  barLinks,
  barScope,
  barSizes,
  barStyleOf,
  bargaugeModel,
  lcdCellCount,
  lcdLitCells,
  namePlacementOf,
  orientationOf,
  reducerOf,
  showUnfilledOf,
  valueModeOf,
  valueTextOf,
  type Bar,
  type BarLimits,
  type BarSizes,
  type BarStyle,
  type NamePlacement,
  type Orientation,
  type ValueMode,
} from "./bargauge-model"

const bodyInsetX = 24
const bodyInsetY = 90
const leftNameShare = 0.5

type BarProps = {
  bar: Bar
  orientation: Orientation
  fields: FieldResolver
  reducer: Reducer
  scope: LinkScope
  placement: NamePlacement
  barStyle: BarStyle
  valueMode: ValueMode
  unfilled: boolean
  cells: number
  limits: BarLimits
  sizes: BarSizes
  runner: StableActionRunner
}

function textSize(size: number | undefined): CSSProperties | undefined {
  return size ? { fontSize: `${size}px` } : undefined
}

function runBarAction(
  { bar, fields, reducer, scope, runner }: BarProps,
  action: FieldAction
) {
  runFieldAction(runner, action, barScope(fields, bar, reducer, scope))
}

function oneClickOf(props: BarProps) {
  const { bar, fields, reducer, scope, valueMode, runner } = props
  if (valueMode === "hidden") return {}
  const links = barLinks(fields, bar, reducer, scope)
  const target = oneClickTarget(links, barActions(fields, bar, reducer, scope))
  if (target === null) return {}
  if (target.kind === "link") return oneClickProps(links)
  if (runner.reason !== null) return {}
  return oneClickProps([], () => runBarAction(props, target.action))
}

function BarActions({
  bar,
  actions,
  reason,
  onAction,
}: {
  bar: Bar
  actions: FieldAction[]
  reason: string | null
  onAction: (action: FieldAction) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="shrink-0 text-muted-foreground"
            aria-label={`Actions for ${bar.name}`}
          />
        }
      >
        <ZapIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        <ActionMenuItems
          actions={actions}
          reason={reason}
          onAction={onAction}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

const oneClickClass = "data-one-click:cursor-pointer"

function Cells({
  bar,
  vertical,
  unfilled,
  cells,
}: Pick<BarProps, "bar" | "unfilled" | "cells"> & { vertical: boolean }) {
  const lit = lcdLitCells(bar.fraction, cells)
  return (
    <span
      className={cn("flex size-full gap-[2px]", vertical && "flex-col-reverse")}
    >
      {Array.from({ length: cells }, (_, index) => (
        <span
          key={index}
          data-part="cell"
          data-lit={index < lit}
          className={cn("flex-1", index >= lit && unfilled && "bg-muted")}
          style={
            index < lit
              ? { backgroundColor: bar.color ?? "var(--series-1)" }
              : undefined
          }
        />
      ))}
    </span>
  )
}

function Track({
  bar,
  orientation,
  barStyle,
  unfilled,
  cells,
}: Pick<BarProps, "bar" | "orientation" | "barStyle" | "unfilled" | "cells">) {
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
        "relative flex overflow-hidden rounded-[2px]",
        unfilled && barStyle === "basic" && "bg-muted",
        vertical
          ? "mx-auto min-h-4 w-full max-w-12 min-w-3 flex-1 flex-col justify-end"
          : "h-2 w-full min-w-0"
      )}
    >
      {barStyle === "lcd" ? (
        <Cells
          bar={bar}
          vertical={vertical}
          unfilled={unfilled}
          cells={cells}
        />
      ) : (
        <span
          data-part="fill"
          className="block"
          style={{
            [vertical ? "height" : "width"]: percent,
            [vertical ? "width" : "height"]: "100%",
            backgroundColor: bar.color ?? "var(--series-1)",
          }}
        />
      )}
    </span>
  )
}

function showsLevel(bar: Bar): boolean {
  return bar.threshold !== "base" && bar.threshold !== "ok"
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

function Value(props: BarProps) {
  const { bar, orientation, fields, reducer, scope, valueMode, sizes, runner } =
    props
  if (valueMode === "hidden")
    return <Level bar={bar} orientation={orientation} />
  const [first] = barLinks(fields, bar, reducer, scope)
  const actions = barActions(fields, bar, reducer, scope)
  const text = <span className="truncate">{bar.text}</span>
  const group = cn(
    "flex min-w-0 items-center text-foreground tabular-nums",
    orientation === "vertical" ? "justify-center gap-1" : "shrink-0 gap-1.5"
  )
  return (
    <span className={group} style={textSize(sizes.valueSize)}>
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
      {actions.length > 0 ? (
        <BarActions
          bar={bar}
          actions={actions}
          reason={runner.reason}
          onAction={(action) => runBarAction(props, action)}
        />
      ) : null}
    </span>
  )
}

function Name({
  bar,
  sizes,
  className,
}: Pick<BarProps, "bar" | "sizes"> & { className?: string }) {
  return (
    <span
      title={bar.name}
      className={cn("truncate", className)}
      style={textSize(sizes.titleSize)}
    >
      {bar.name}
    </span>
  )
}

const HorizontalRow = memo(function HorizontalRow(props: BarProps) {
  const { bar, placement, limits, sizes, valueMode } = props
  const grows = limits.minHeight !== undefined || limits.maxHeight !== undefined
  const style = grows
    ? { minHeight: limits.minHeight, maxHeight: limits.maxHeight }
    : undefined
  const rowClass = cn(
    "flex text-[13px]",
    grows ? "flex-1 basis-0 justify-center" : "shrink-0"
  )
  if (placement === "left") {
    return (
      <div
        {...oneClickOf(props)}
        className={cn(rowClass, "flex-row items-center gap-2", oneClickClass)}
        style={style}
      >
        <Name bar={bar} sizes={sizes} className="w-[30%] max-w-48 shrink-0" />
        <div className="min-w-0 flex-1">
          <Track {...props} />
        </div>
        <Value {...props} />
      </div>
    )
  }
  const showName = placement !== "hidden"
  const showLine = showName || valueMode !== "hidden" || showsLevel(bar)
  return (
    <div
      {...oneClickOf(props)}
      className={cn(rowClass, "flex-col gap-[5px]", oneClickClass)}
      style={style}
    >
      {showLine ? (
        <div
          className={cn(
            "flex items-center gap-2",
            showName ? "justify-between" : "justify-end"
          )}
        >
          {showName ? (
            <Name bar={bar} sizes={sizes} className="min-w-0" />
          ) : null}
          <Value {...props} />
        </div>
      ) : null}
      <Track {...props} />
    </div>
  )
})

const VerticalColumn = memo(function VerticalColumn(props: BarProps) {
  const { bar, placement, limits, sizes } = props
  const name = <Name bar={bar} sizes={sizes} className="text-center" />
  return (
    <div
      {...oneClickOf(props)}
      className={cn(
        "flex min-w-8 flex-1 basis-0 flex-col gap-1 text-[13px]",
        oneClickClass
      )}
      style={{
        minWidth: limits.minWidth,
        minHeight: limits.minHeight,
        maxHeight: limits.maxHeight,
      }}
    >
      {placement === "above" ? name : null}
      <Value {...props} />
      <Track {...props} />
      {placement === "below" ? name : null}
    </div>
  )
})

const BargaugePanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display,
  zone,
  meta,
  links,
  width,
  height,
}) => {
  const { runner, dialog } = useStableActionRunner(panel.id, meta)
  const model = useMemo(
    () => bargaugeModel(frames, fields, display, zone),
    [frames, fields, display, zone]
  )
  const reducer = reducerOf(display)
  const orientation = orientationOf(display, { width, height })
  const vertical = orientation === "vertical"
  const Item = vertical ? VerticalColumn : HorizontalRow
  const placement = namePlacementOf(display, orientation)
  const extent = vertical
    ? height - bodyInsetY
    : (width - bodyInsetX) * (placement === "left" ? leftNameShare : 1)
  const shared = {
    orientation,
    placement,
    barStyle: barStyleOf(display),
    valueMode: valueModeOf(display),
    unfilled: showUnfilledOf(display),
    cells: lcdCellCount(extent),
    limits: barLimits(display),
    sizes: barSizes(display),
    fields,
    reducer,
    scope: links,
    runner,
  }
  return (
    <>
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
          <Item key={bar.key} bar={bar} {...shared} />
        ))}
      </div>
      {dialog}
    </>
  )
}

export default BargaugePanel
