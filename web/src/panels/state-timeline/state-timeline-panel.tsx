import { useMemo } from "react"

import type { PanelRenderer } from "../types"
import { TimelineChart } from "./timeline-chart"
import { timelineModel } from "./timeline-model"

const StateTimelinePanel: PanelRenderer = ({
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
    () => timelineModel(frames, fields, display, { from, to }),
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
      variant="segments"
    />
  )
}

export default StateTimelinePanel
