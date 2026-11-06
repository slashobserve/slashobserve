import { memo, useEffect, useMemo, useRef } from "react"

import type { Selections } from "@/api/keys"
import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import { stateScope, type PanelSource } from "@/dashboards/panel-source"
import { useInView } from "@/dashboards/use-in-view"
import { maxDataPointsFor, usePanelData } from "@/dashboards/use-panel-data"
import { VariableLoadError } from "@/dashboards/variables"
import { isRenderable, isStaticPanel, preloadRenderer } from "@/panels"
import { createFieldResolver } from "@/panels/field-config"
import { PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import type { SharedCursor } from "@/panels/shared-cursor"
import { reusedFilterOf, reusePanel } from "@/panels/transforms/reuse-panel"
import {
  isDataPanel,
  panelTitle,
  type DataPanel,
  type LayoutPanel,
  type Variable,
} from "@/spec"

import { availableOnPublicLink } from "./supported"

type Props = {
  source: PanelSource
  cursor: SharedCursor
  panel: LayoutPanel
  reused: DataPanel | undefined
  variables: readonly Variable[]
  title: string
  width: number
  height: number
  from: string | undefined
  to: string | undefined
  timezone: string | undefined
  rangeFrom: string
  rangeTo: string
  selections: Selections
  selectionsReady: boolean
  blockedBy: string | undefined
  onRetryBlocked: () => void
  refreshMs: number | null
  onTimeRangeSelect: (fromMs: number, toMs: number) => void
}

export const PublicPanel = memo(function PublicPanel({
  source,
  cursor,
  panel: sourcePanel,
  reused,
  variables,
  title,
  width,
  height,
  from,
  to,
  timezone,
  rangeFrom,
  rangeTo,
  selections,
  selectionsReady,
  blockedBy,
  onRetryBlocked,
  refreshMs,
  onTimeRangeSelect,
}: Props) {
  const panel = useMemo(
    () =>
      title === panelTitle(sourcePanel)
        ? sourcePanel
        : { ...sourcePanel, title },
    [sourcePanel, title]
  )
  const ref = useRef<HTMLDivElement>(null)
  const { seen, inView } = useInView(ref)
  const body = panelBodySize(Math.floor(width), Math.floor(height))
  const available = availableOnPublicLink(panel.type)
  const supported = available && isRenderable(panel.type)
  const dataPanel = useMemo(
    () => (isDataPanel(panel) ? reusePanel(panel, reused) : undefined),
    [panel, reused]
  )
  const reusedFilter = useMemo(
    () =>
      isDataPanel(panel)
        ? reusedFilterOf(panel, reused, variables, selections)
        : undefined,
    [panel, reused, variables, selections]
  )

  useEffect(() => {
    if (seen && supported) preloadRenderer(panel.type)
  }, [seen, supported, panel.type])

  const query = usePanelData({
    source,
    panel: dataPanel ?? panel,
    from,
    to,
    timezone,
    vars: selections,
    maxDataPoints: maxDataPointsFor(dataPanel ?? panel, body.width),
    refreshMs,
    enabled: dataPanel !== undefined && supported && seen && selectionsReady,
    inView,
  })
  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel, selections) : undefined),
    [dataPanel, selections]
  )
  const populated = useMemo(
    () =>
      dataPanel !== undefined &&
      fields !== undefined &&
      query.data !== undefined &&
      panelHasData(
        dataPanel,
        query.data.frames,
        fields,
        reusedFilter,
        selections
      ),
    [dataPanel, fields, query.data, reusedFilter, selections]
  )
  const blockedError = useMemo(
    () =>
      blockedBy === undefined ? undefined : new VariableLoadError(blockedBy),
    [blockedBy]
  )

  let state: PanelState = "loading"
  if (!available) state = "ready"
  else if (!supported) state = "unsupported"
  else if (!dataPanel) state = "ready"
  else if (query.data) state = populated ? "ready" : "empty"
  else if (blockedBy !== undefined) state = "error"
  else if (query.isError) state = "error"

  return (
    <div ref={ref} className="size-full">
      <PanelChrome
        panel={panel}
        state={state}
        error={query.data ? query.error : (blockedError ?? query.error)}
        refreshing={query.isFetching && query.data !== undefined}
        staleError={query.isError && query.data ? query.error : undefined}
        onRetry={() => (blockedError ? onRetryBlocked() : void query.refetch())}
      >
        {available ? null : (
          <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
            Not available on a public link
          </div>
        )}
        {dataPanel && query.data && supported ? (
          <PanelBody
            panel={dataPanel}
            data={query.data}
            width={body.width}
            height={body.height}
            stateScope={stateScope(source)}
            cursor={cursor}
            onTimeRangeSelect={onTimeRangeSelect}
            vars={selections}
            timezone={timezone}
            from={rangeFrom}
            to={rangeTo}
            reusedFilter={reusedFilter}
          />
        ) : null}
        {isStaticPanel(panel) && supported ? (
          <StaticPanelBody
            panel={panel}
            width={body.width}
            height={body.height}
            vars={selections}
            from={rangeFrom}
            to={rangeTo}
          />
        ) : null}
      </PanelChrome>
    </div>
  )
})
