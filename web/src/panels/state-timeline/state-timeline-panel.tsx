import { useMemo } from "react"

import type { PanelRenderer } from "../types"
import { TimelineChart } from "./timeline-chart"
import { timelineModel } from "./timeline-model"

const StateTimelinePanel: PanelRenderer = ({
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
  annotations,
}) => {
  const from = Date.parse(meta.from)
  const to = Date.parse(meta.to)
  const model = useMemo(
    () => timelineModel(frames, fields, display, { from, to }, meta),
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
      variant="segments"
      cursor={cursor}
      stateKey={`${stateScope}:${panel.id}`}
      zone={zone}
      annotations={annotations}
    />
  )
}

export default StateTimelinePanel
