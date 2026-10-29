import type { Dispatch } from "react"

import { dataPanelTypes, type DataPanelType } from "@/spec"

import type { DraftAction } from "./draft"
import { SelectField } from "./field-controls"

const typeLabels: Record<DataPanelType, string> = {
  timeseries: "Time series",
  stat: "Stat",
  gauge: "Gauge",
  bargauge: "Bar gauge",
  table: "Table",
  piechart: "Pie chart",
  heatmap: "Heatmap",
  histogram: "Histogram",
  "state-timeline": "State timeline",
  logs: "Logs",
  barchart: "Bar chart",
  "status-history": "Status history",
  trend: "Trend",
  xychart: "XY chart",
  candlestick: "Candlestick",
  nodeGraph: "Node graph",
  traces: "Traces",
  flamegraph: "Flame graph",
  geomap: "Geomap",
  canvas: "Canvas",
  alertlist: "Alert list",
}

const options = dataPanelTypes.map((type) => ({
  value: type,
  label: typeLabels[type],
}))

export function TypeSelect({
  type,
  dispatch,
  readOnly,
}: {
  type: DataPanelType
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}) {
  return (
    <SelectField
      label="Panel type"
      value={type}
      options={options}
      disabled={readOnly}
      onChange={(value) => {
        const panelType = dataPanelTypes.find((item) => item === value)
        if (panelType) dispatch({ type: "setType", panelType })
      }}
    />
  )
}
