import { useMemo } from "react"

import { stableJson } from "@/lib/json"

import { dataRenderers } from "../index"
import { alignFrames } from "../timeseries/align"
import { shapeNulls } from "../timeseries/gaps"
import { TimeseriesChart } from "../timeseries/timeseries-panel"
import type { PanelRenderer } from "../types"
import { Unsupported } from "../unsupported"
import {
  framesWithX,
  noXFieldReason,
  tableFallbackNote,
  tableFallbackNoteHeight,
  trendSource,
  xAxisFor,
  xFieldName,
} from "./trend-model"

const TrendPanel: PanelRenderer = (props) => {
  const { frames, fields, display, meta } = props
  const xField = xFieldName(display)
  const source = useMemo(() => trendSource(xField), [xField])
  const usable = useMemo(() => framesWithX(frames, source), [frames, source])
  const placed = useMemo(
    () => alignFrames(usable, fields.displayName, [], source),
    [usable, fields, source]
  )
  const configs = useMemo(
    () =>
      placed.series.map(({ frame, field }) => fields.configFor(frame, field)),
    [placed.series, fields]
  )
  const aligned = useMemo(
    () => shapeNulls(placed, configs, meta),
    [placed, configs, meta]
  )
  const xConfigJson = useMemo(() => {
    const frame = usable[0]
    const field = frame && source.fieldOf(frame)
    return frame && field ? stableJson(fields.configFor(frame, field)) : null
  }, [usable, source, fields])
  const xAxis = useMemo(() => xAxisFor(xConfigJson), [xConfigJson])

  if (usable.length === 0 || !xAxis) {
    return (
      <p className="m-0 p-3 text-xs text-muted-foreground">{noXFieldReason}</p>
    )
  }
  if (aligned.series.length === 0 || aligned.xs.length === 0) {
    const Table = dataRenderers.table
    if (!Table) return <Unsupported />
    return (
      <div className="flex size-full flex-col">
        <p
          className="m-0 shrink-0 overflow-y-auto px-3 pt-2 text-xs text-muted-foreground"
          style={{ height: tableFallbackNoteHeight }}
        >
          {tableFallbackNote}
        </p>
        <Table {...props} height={props.height - tableFallbackNoteHeight} />
      </div>
    )
  }
  return (
    <TimeseriesChart
      {...props}
      aligned={aligned}
      configs={configs}
      xAxis={xAxis}
    />
  )
}

export default TrendPanel
