import { useMemo } from "react"

import { TimelineChart } from "../state-timeline/timeline-chart"
import { timelineModel } from "../state-timeline/timeline-model"
import type { PanelRenderer } from "../types"

const StatusHistoryPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  zone,
  meta,
  width,
  height,
  cursor,
  stateScope,
  panel,
}) => {
  const from = Date.parse(meta.from)
  const to = Date.parse(meta.to)
  const model = useMemo(
    () => timelineModel(frames, fields, display, { from, to }, meta, true),
    [frames, fields, display, from, to, meta]
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
      cursor={cursor}
      stateKey={`${stateScope}:${panel.id}`}
      zone={zone}
    />
  )
}

export default StatusHistoryPanel
