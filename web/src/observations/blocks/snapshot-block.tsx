import { useQuery } from "@tanstack/react-query"
import { useMemo, useState, type ReactNode } from "react"
import { useContainerWidth } from "react-grid-layout"

import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import { isRenderable } from "@/panels"
import { createFieldResolver } from "@/panels/field-config"
import { PanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import { createSharedCursor } from "@/panels/shared-cursor"
import type { Block } from "@/observations/queries"
import { isDataPanel, type LayoutPanel } from "@/spec"

import { BlockByline } from "./block-byline"
import {
  blockPanelId,
  frozenCaption,
  snapshotIdOf,
  snapshotQuery,
} from "./block-data"

const noVars = {}

const unreadablePanel = {
  id: blockPanelId,
  type: "text",
  title: "Snapshot",
  content: "",
} as unknown as LayoutPanel

function ignoreRangeSelect() {}

type Props = { block: Block; height: number }

function SnapshotFrame({
  block,
  caption,
  children,
}: {
  block: Block
  caption?: string
  children: ReactNode
}) {
  return (
    <div className="flex w-full flex-col gap-2">
      {children}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-0.5">
        <span className="min-w-0 truncate text-[12px] text-muted-foreground">
          {caption}
        </span>
        <BlockByline block={block} />
      </div>
    </div>
  )
}

function UnreadableSnapshot({ block, height }: Props) {
  return (
    <SnapshotFrame block={block}>
      <div style={{ height }} className="w-full">
        <PanelChrome panel={unreadablePanel} state="ready">
          <div className="flex size-full items-center justify-center p-3 text-center text-sm text-muted-foreground">
            This block could not be read.
          </div>
        </PanelChrome>
      </div>
    </SnapshotFrame>
  )
}

function ReadableSnapshotBlock({
  block,
  snapshotId,
  height,
}: Props & { snapshotId: string }) {
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const [cursor] = useState(createSharedCursor)
  const query = useQuery(snapshotQuery(snapshotId))
  const snapshot = query.data
  const panel = useMemo(
    () =>
      snapshot === undefined
        ? undefined
        : ({
            ...snapshot.panel,
            id: blockPanelId,
            queries: [],
          } as unknown as LayoutPanel),
    [snapshot]
  )
  const dataPanel =
    panel !== undefined && isDataPanel(panel) ? panel : undefined
  const supported = panel !== undefined && isRenderable(panel.type)
  const size = panelBodySize(Math.floor(width), Math.floor(height))
  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel, noVars) : undefined),
    [dataPanel]
  )
  const populated =
    dataPanel !== undefined &&
    fields !== undefined &&
    snapshot !== undefined &&
    panelHasData(dataPanel, snapshot.data.frames, fields)

  let state: PanelState = "loading"
  if (query.isError) state = "error"
  else if (snapshot !== undefined) {
    if (!supported || !dataPanel) state = "unsupported"
    else state = populated ? "ready" : "empty"
  }

  const caption =
    snapshot === undefined
      ? undefined
      : frozenCaption(snapshot.time_from, snapshot.time_to)

  return (
    <SnapshotFrame block={block} caption={caption}>
      <div ref={containerRef} style={{ height }} className="w-full">
        {mounted ? (
          <PanelChrome
            panel={panel ?? unreadablePanel}
            state={state}
            error={query.error}
            onRetry={() => void query.refetch()}
          >
            {dataPanel && snapshot ? (
              <PanelBody
                panel={dataPanel}
                data={snapshot.data}
                width={size.width}
                height={size.height}
                stateScope={`block:${block.id}`}
                cursor={cursor}
                onTimeRangeSelect={ignoreRangeSelect}
                vars={noVars}
                timezone={undefined}
                from={snapshot.time_from}
                to={snapshot.time_to}
              />
            ) : null}
          </PanelChrome>
        ) : null}
      </div>
    </SnapshotFrame>
  )
}

export function SnapshotBlock({ block, height }: Props) {
  const snapshotId = snapshotIdOf(block)
  if (snapshotId === null) {
    return <UnreadableSnapshot block={block} height={height} />
  }
  return (
    <ReadableSnapshotBlock
      block={block}
      snapshotId={snapshotId}
      height={height}
    />
  )
}
