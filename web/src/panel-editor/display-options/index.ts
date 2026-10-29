import type { DisplayPanelType } from "@/spec"

import { alertlist } from "./alertlist"
import { bargauge } from "./bargauge"
import { barchart } from "./barchart"
import { candlestick } from "./candlestick"
import { canvas } from "./canvas"
import { dashlist } from "./dashlist"
import { flamegraph } from "./flamegraph"
import { gauge } from "./gauge"
import { geomap } from "./geomap"
import { heatmap } from "./heatmap"
import { histogram } from "./histogram"
import { logs } from "./logs"
import { news } from "./news"
import { nodeGraph } from "./nodeGraph"
import { piechart } from "./piechart"
import { stat } from "./stat"
import { stateTimeline } from "./state-timeline"
import { statusHistory } from "./status-history"
import { table } from "./table"
import { timeseries } from "./timeseries"
import { traces } from "./traces"
import { trend } from "./trend"
import type { DisplayOption } from "./types"
import { xychart } from "./xychart"

export const displayOptions: Record<
  DisplayPanelType,
  readonly DisplayOption[]
> = {
  timeseries,
  stat,
  gauge,
  bargauge,
  table,
  piechart,
  heatmap,
  histogram,
  "state-timeline": stateTimeline,
  logs,
  barchart,
  "status-history": statusHistory,
  trend,
  xychart,
  candlestick,
  nodeGraph,
  traces,
  flamegraph,
  geomap,
  canvas,
  alertlist,
  dashlist,
  news,
}
