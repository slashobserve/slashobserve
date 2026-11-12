import { useMemo, useState, type ReactNode } from "react"
import { useContainerWidth } from "react-grid-layout"

import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import { maxDataPointsFor } from "@/dashboards/use-panel-data"
import { useDatasources } from "@/datasources/queries"
import { isRenderable, isStaticPanel } from "@/panels"
import { createFieldResolver } from "@/panels/field-config"
import { PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import { createSharedCursor } from "@/panels/shared-cursor"
import type { Block } from "@/observations/queries"
import { isDataPanel, isRowPanel, type LayoutPanel } from "@/spec"

import { BlockByline } from "./block-byline"
import {
  blockPanelId,
  datasourceUidOf,
  panelBlockBody,
  useBlockPanelData,
  type PanelBlockBody,
} from "./block-data"

const noVars = {}

const defaultFrom = "now-1h"

const defaultTo = "now"

const unreadablePanel = {
  id: blockPanelId,
  type: "text",
  title: "Panel",
  content: "",
} as unknown as LayoutPanel

function ignoreRangeSelect() {}

type Props = { block: Block; height: number }

function LiveCaption({ datasource }: { datasource: string | undefined }) {
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-[12px] text-muted-foreground">
      <span
        aria-hidden="true"
        className="size-1.5 flex-none rounded-full bg-status-ok"
      />
      <span className="truncate">
        {datasource === undefined ? "live" : `${datasource}, live`}
      </span>
    </span>
  )
}

function BlockFrame({
  block,
  caption,
  children,
}: {
  block: Block
  caption?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex w-full flex-col gap-2">
      {children}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-0.5">
        {caption ?? <span />}
        <BlockByline block={block} />
      </div>
    </div>
  )
}

function ReadablePanelBlock({
  block,
  body,
  height,
}: {
  block: Block
  body: PanelBlockBody
  height: number
}) {
  const { panel, from, to } = body
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const [cursor] = useState(createSharedCursor)
  const datasources = useDatasources()
  const dataPanel = isDataPanel(panel) ? panel : undefined
  const supported = !isRowPanel(panel) && isRenderable(panel.type)
  const size = panelBodySize(Math.floor(width), Math.floor(height))
  const query = useBlockPanelData({
    blockId: block.id,
    from,
    to,
    maxDataPoints: maxDataPointsFor(panel, size.width),
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
  const datasourceUid = dataPanel ? datasourceUidOf(dataPanel) : undefined
  const datasourceName =
    datasourceUid === undefined
      ? undefined
      : (datasources.data?.find((entry) => entry.uid === datasourceUid)?.name ??
        datasourceUid)

  if (isRowPanel(panel)) return null

  let state: PanelState = "loading"
  if (!supported) state = "unsupported"
  else if (!dataPanel) state = "ready"
  else if (query.data) state = populated ? "ready" : "empty"
  else if (query.isError) state = "error"

  return (
    <BlockFrame
      block={block}
      caption={<LiveCaption datasource={datasourceName} />}
    >
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
                width={size.width}
                height={size.height}
                stateScope={`block:${block.id}`}
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
                width={size.width}
                height={size.height}
                vars={noVars}
                from={from ?? defaultFrom}
                to={to ?? defaultTo}
              />
            ) : null}
          </PanelChrome>
        ) : null}
      </div>
    </BlockFrame>
  )
}

export function PanelBlock({ block, height }: Props) {
  const body = useMemo(() => panelBlockBody(block), [block])
  if (body === null) {
    return (
      <BlockFrame block={block}>
        <div style={{ height }} className="w-full">
          <PanelChrome panel={unreadablePanel} state="ready">
            <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
              This block could not be read.
            </div>
          </PanelChrome>
        </div>
      </BlockFrame>
    )
  }
  return <ReadablePanelBlock block={block} body={body} height={height} />
}
