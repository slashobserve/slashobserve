import { Link } from "@tanstack/react-router"
import { CompassIcon } from "lucide-react"
import { useCallback, useMemo, useState } from "react"
import { useContainerWidth } from "react-grid-layout"

import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import { exploreTarget } from "@/dashboards/explore-link"
import { maxDataPointsFor } from "@/dashboards/use-panel-data"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { isRenderable, isStaticPanel } from "@/panels"
import { createFieldResolver, type FieldResolver } from "@/panels/field-config"
import { PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import { createSharedCursor } from "@/panels/shared-cursor"
import type { PreparedFrame } from "@/panels/transforms"
import { isDataPanel, isRowPanel, type Panel } from "@/spec"

import { useFindingPanelData } from "./queries"
import { traceColumnOverrides } from "./trace-columns"

const noVars = {}

function ignoreRangeSelect() {}

function sameRef(ref: string) {
  return ref
}

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
  const datasources = useDatasources().data
  const executed = useCallback(
    (ref: string) =>
      query.data?.meta.queries?.find((entry) => entry.ref === ref)?.executed,
    [query.data]
  )
  const explore = useMemo(
    () =>
      dataPanel === undefined
        ? undefined
        : exploreTarget(
            dataPanel,
            sameRef,
            datasources === undefined
              ? undefined
              : (uid) => datasourceTypeOf(datasources, uid),
            { from, to },
            executed
          ),
    [dataPanel, datasources, from, to, executed]
  )
  const columnOverrides = useMemo(
    () =>
      dataPanel && datasources
        ? (frames: readonly PreparedFrame[], resolver: FieldResolver) =>
            traceColumnOverrides(dataPanel, frames, datasources, resolver)
        : undefined,
    [dataPanel, datasources]
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
          menu={
            explore !== undefined && "opening" in explore ? (
              <Link
                {...explore.opening.target}
                title={explore.opening.label}
                className={cn(
                  buttonVariants({ variant: "ghost", size: "xs" }),
                  "text-muted-foreground"
                )}
              >
                <CompassIcon aria-hidden="true" />
                Open in Explore
              </Link>
            ) : undefined
          }
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
              columnOverrides={columnOverrides}
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
