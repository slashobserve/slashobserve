import { useMemo, useState } from "react"
import { useContainerWidth } from "react-grid-layout"

import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import { maxDataPointsFor } from "@/dashboards/use-panel-data"
import { isRenderable, isStaticPanel } from "@/panels"
import { createFieldResolver } from "@/panels/field-config"
import { PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import { createSharedCursor } from "@/panels/shared-cursor"
import { isDataPanel, isRowPanel, type Panel } from "@/spec"

import { useFindingPanelData } from "./queries"

const noVars = {}

function ignoreRangeSelect() {}

type Props = {
  findingId: string
  panel: Panel
  from: string
  to: string
  height: number
}

export function EvidencePanel({ findingId, panel, from, to, height }: Props) {
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const [cursor] = useState(createSharedCursor)
  const dataPanel = isDataPanel(panel) ? panel : undefined
  const supported = !isRowPanel(panel) && isRenderable(panel.type)
  const body = panelBodySize(Math.floor(width), Math.floor(height))
  const query = useFindingPanelData({
    findingId,
    panel,
    maxDataPoints: maxDataPointsFor(panel, body.width),
    enabled: dataPanel !== undefined && supported && mounted,
  })
  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel, noVars) : undefined),
    [dataPanel]
  )
  const populated = useMemo(
    () =>
      dataPanel !== undefined &&
      fields !== undefined &&
      query.data !== undefined &&
      panelHasData(dataPanel, query.data.frames, fields),
    [dataPanel, fields, query.data]
  )

  if (isRowPanel(panel)) return null

  let state: PanelState = "loading"
  if (!supported) state = "unsupported"
  else if (!dataPanel) state = "ready"
  else if (query.data) state = populated ? "ready" : "empty"
  else if (query.isError) state = "error"

  return (
    <div ref={containerRef} style={{ height }} className="w-full">
      {mounted ? (
        <PanelChrome
          panel={panel}
          state={state}
          error={query.error}
          refreshing={query.isFetching && query.data !== undefined}
          onRetry={() => void query.refetch()}
        >
          {dataPanel && query.data ? (
            <PanelBody
              panel={dataPanel}
              data={query.data}
              width={body.width}
              height={body.height}
              stateScope={`finding:${findingId}`}
              cursor={cursor}
              onTimeRangeSelect={ignoreRangeSelect}
              vars={noVars}
              timezone={undefined}
              from={from}
              to={to}
            />
          ) : null}
          {isStaticPanel(panel) && supported ? (
            <StaticPanelBody
              panel={panel}
              width={body.width}
              height={body.height}
              vars={noVars}
              from={from}
              to={to}
            />
          ) : null}
        </PanelChrome>
      ) : null}
    </div>
  )
}
