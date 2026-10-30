import { useCallback, useLayoutEffect, useMemo, type ReactNode } from "react"

import type { Reducer } from "@/spec"

import type { FieldConfig } from "../field-config"
import { reducerLabels } from "../reducers"
import { legendCells } from "../timeseries/plot-model"
import {
  emptySeriesState,
  useSeriesStore,
  visibleSeries,
} from "../timeseries/series-state"
import {
  Legend,
  rootFontSize,
  type LegendHover,
  type LegendItem,
  type LegendPlacement,
} from "./legend"
import { effectiveLegendMode, legendOrder } from "./legend-model"
import { LegendTable, useLegendTextMeasure } from "./legend-table"
import { legendTableWidth, rightLegendWidth } from "./legend-width"

export type SeriesLegendEntry = {
  key: string
  label: string
  color: string
  values: readonly unknown[]
  config: FieldConfig
  hideInLegend: boolean
}

export type SeriesLegendOptions = {
  legend: "bottom" | "right" | "hidden"
  legendMode?: "list" | "table"
  legendValues: Reducer[]
  legendSortBy?: "name" | Reducer
  legendSortDesc: boolean
  legendWidth?: number
}

export type SeriesLegend = {
  hidden: ReadonlySet<string>
  placement: LegendPlacement | null
  width: number
  node: ReactNode
}

const legendShare = 0.4

export function useSeriesLegend({
  stateKey,
  entries,
  options,
  width,
  height,
  onHover,
}: {
  stateKey: string
  entries: readonly SeriesLegendEntry[]
  options: SeriesLegendOptions
  width: number
  height: number
  onHover?: LegendHover
}): SeriesLegend {
  const keys = useMemo(() => entries.map(({ key }) => key), [entries])
  const stored = useSeriesStore((store) => store.panels[stateKey])
  const commitKeys = useSeriesStore((store) => store.commit)
  const toggleKey = useSeriesStore((store) => store.toggle)
  const storedHidden = (stored ?? emptySeriesState).hidden

  const reducers = options.legendValues
  const mode = effectiveLegendMode(options.legendMode, reducers)
  const order = useMemo(
    () =>
      legendOrder(
        entries.map(({ key, label, values, hideInLegend }) => ({
          key,
          label,
          values,
          hidden: hideInLegend,
        })),
        {
          mode,
          values: reducers,
          sortBy: options.legendSortBy,
          sortDesc: options.legendSortDesc,
          hideZero: false,
          hideEmpty: false,
        }
      ),
    [entries, mode, reducers, options.legendSortBy, options.legendSortDesc]
  )
  const listed = useMemo(
    () => order.map((index) => entries[index]),
    [order, entries]
  )
  const shown =
    entries.length > 1 && options.legend !== "hidden" && listed.length > 0
  const hidden = useMemo(
    () => visibleSeries(storedHidden, keys, shown),
    [storedHidden, keys, shown]
  )

  useLayoutEffect(() => {
    commitKeys(stateKey, keys)
  }, [commitKeys, stateKey, keys])

  const toggle = useCallback(
    (key: string) => toggleKey(stateKey, key),
    [toggleKey, stateKey]
  )

  const cells = useMemo(
    () =>
      legendCells(
        listed.map(({ values, config }) => ({ fieldValues: values, config })),
        reducers
      ),
    [listed, reducers]
  )
  const textMeasure = useLegendTextMeasure()
  const placement: LegendPlacement | null = shown
    ? options.legend === "right"
      ? "right"
      : "bottom"
    : null
  const tableWidth = useMemo(() => {
    if (placement !== "right" || reducers.length === 0) return null
    const rem = rootFontSize()
    return legendTableWidth(
      listed.map(({ label }) => label),
      reducers.map((reducer) => reducerLabels[reducer]),
      cells,
      textMeasure(rem),
      rem
    )
  }, [placement, listed, reducers, cells, textMeasure])
  const legendWidth =
    placement === "right"
      ? rightLegendWidth(tableWidth, width, options.legendWidth)
      : 0

  const items: LegendItem[] = listed.map((entry) => ({
    key: entry.key,
    label: entry.label,
    color: entry.color,
    line: "solid",
    style: "bars",
  }))
  const common = {
    items,
    reducers,
    cells,
    hidden,
    available: height * legendShare,
    width: legendWidth,
    onToggle: toggle,
    onHover,
  }
  let node: ReactNode = null
  if (placement !== null) {
    node =
      mode === "table" ? (
        <LegendTable
          {...common}
          placement={placement}
          sortBy={options.legendSortBy}
          sortDesc={options.legendSortDesc}
        />
      ) : (
        <Legend {...common} placement={placement} />
      )
  }
  return { hidden, placement, width: legendWidth, node }
}
