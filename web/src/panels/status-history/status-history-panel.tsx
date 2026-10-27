import { useMemo } from "react"

import { TimelineChart } from "../state-timeline/timeline-chart"
import { timelineModel } from "../state-timeline/timeline-model"
import type { PanelRenderer } from "../types"

export const StatusHistoryPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  meta,
  width,
  height,
}) => {
  const from = Date.parse(meta.from)
  const to = Date.parse(meta.to)
  const model = useMemo(
    () => timelineModel(frames, fields, display, { from, to }, true),
    [frames, fields, display, from, to]
  )
  return (
    <TimelineChart
      model={model}
      display={display}
      from={from}
      to={to}
      width={width}
      height={height}
      variant="cells"
    />
  )
}

export default StatusHistoryPanel
