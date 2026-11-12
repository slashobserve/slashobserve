import { useMemo, useState } from "react"
import { useQuery, useSuspenseQuery } from "@tanstack/react-query"
import { useContainerWidth } from "react-grid-layout"

import { Wordmark } from "@/components/brand/wordmark"
import { LoadProblem } from "@/components/load-problem"
import {
  panelBodySize,
  PanelChrome,
  type PanelState,
} from "@/dashboards/panel-chrome"
import {
  type DatasourceRef,
  DatasourceListProvider,
} from "@/datasources/queries"
import { FindingMarkdown } from "@/findings/finding-markdown"
import { formatDateTime } from "@/lib/time"
import { useDocumentTitle } from "@/lib/document-title"
import { frozenCaption } from "@/observations/blocks/block-data"
import { isRenderable } from "@/panels"
import { createFieldResolver } from "@/panels/field-config"
import { PanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import { createSharedCursor } from "@/panels/shared-cursor"
import { availableOnPublicLink } from "@/public/supported"
import { loadProblemText } from "@/service-accounts/queries"
import { isDataPanel, type LayoutPanel } from "@/spec"

import {
  publicRcaQuery,
  publicRcaSnapshotQuery,
  type PublicRca,
  type RcaBlock,
} from "./queries"

const noDatasources: readonly DatasourceRef[] = []
const noVars = {}
const panelId = "block"
const halfHeight = 220
const fullHeight = 280

type Snapshot = Extract<RcaBlock, { kind: "snapshot" }>

function ignoreRangeSelect() {}

export function publicRows(blocks: readonly RcaBlock[]): RcaBlock[][] {
  const rows: RcaBlock[][] = []
  for (let i = 0; i < blocks.length; i++) {
    const block = blocks[i]
    const next = blocks[i + 1]
    if (block.width === "half" && next !== undefined && next.width === "half") {
      rows.push([block, next])
      i++
    } else {
      rows.push([block])
    }
  }
  return rows
}

function rangeCaption(rca: PublicRca): string {
  return rca.from !== null && rca.to !== null
    ? frozenCaption(rca.from, rca.to)
    : "Frozen when published"
}

function FrozenPanel({
  token,
  item,
  caption,
}: {
  token: string
  item: Snapshot
  caption: string
}) {
  const height = item.width === "half" ? halfHeight : fullHeight
  const { width, mounted, containerRef } = useContainerWidth({
    measureBeforeMount: true,
  })
  const [cursor] = useState(createSharedCursor)
  const query = useQuery(publicRcaSnapshotQuery(token, item.snapshot_id))
  const frozen = query.data
  const panel = useMemo(
    () =>
      ({
        ...item.panel,
        id: panelId,
        title: item.title,
        queries: [],
      }) as unknown as LayoutPanel,
    [item.panel, item.title]
  )
  const dataPanel = isDataPanel(panel) ? panel : undefined
  const supported =
    availableOnPublicLink(panel.type) && isRenderable(panel.type)
  const size = panelBodySize(Math.floor(width), Math.floor(height))
  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel, noVars) : undefined),
    [dataPanel]
  )
  const populated =
    dataPanel !== undefined &&
    fields !== undefined &&
    frozen !== undefined &&
    panelHasData(dataPanel, frozen.frames, fields)

  let state: PanelState = "loading"
  if (!supported || !dataPanel) state = "unsupported"
  else if (query.isError) state = "error"
  else if (frozen !== undefined) state = populated ? "ready" : "empty"

  return (
    <div className="flex w-full flex-col gap-2">
      <div ref={containerRef} style={{ height }} className="w-full">
        {mounted ? (
          <PanelChrome
            panel={panel}
            state={state}
            error={query.error}
            onRetry={() => void query.refetch()}
          >
            {supported && dataPanel && frozen ? (
              <PanelBody
                panel={dataPanel}
                data={frozen}
                width={size.width}
                height={size.height}
                stateScope={`rca:${item.snapshot_id}`}
                cursor={cursor}
                onTimeRangeSelect={ignoreRangeSelect}
                vars={noVars}
                timezone={undefined}
                from={frozen.meta.from}
                to={frozen.meta.to}
              />
            ) : null}
          </PanelChrome>
        ) : null}
      </div>
      <p className="px-0.5 text-[12px] text-muted-foreground">{caption}</p>
    </div>
  )
}

function RcaItem({
  token,
  item,
  caption,
}: {
  token: string
  item: RcaBlock
  caption: string
}) {
  if (item.kind === "text") return <FindingMarkdown markdown={item.markdown} />
  return <FrozenPanel token={token} item={item} caption={caption} />
}

export function PublicRcaPage({ token }: { token: string }) {
  const rca = useSuspenseQuery(publicRcaQuery(token)).data
  useDocumentTitle(`${rca.title} - SlashObserve`)
  const caption = rangeCaption(rca)
  const rows = publicRows(rca.blocks)
  return (
    <DatasourceListProvider value={noDatasources}>
      <div className="flex min-h-svh flex-col bg-background text-foreground">
        <header className="pt-3.5 pr-4 pb-2.5 pl-4">
          <Wordmark />
        </header>
        <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-4 pt-8 pb-16">
          <div className="flex flex-col gap-3">
            <h1 className="text-[28px] leading-9 font-semibold tracking-[-0.01em]">
              {rca.title}
            </h1>
            <p className="text-[13px] text-muted-foreground">
              Revision {rca.revision}, published by {rca.published_by},{" "}
              {formatDateTime(rca.created_at)}
            </p>
            <p className="text-[13px] font-medium">
              {rca.observation_state === "resolved"
                ? "Resolved"
                : "Open at publish time"}
            </p>
            {rca.summary === null || rca.summary.trim() === "" ? null : (
              <p className="text-[15px]">{rca.summary}</p>
            )}
          </div>
          <ol className="flex flex-col gap-6">
            {rows.map((row, index) => (
              <li key={index} className="@container">
                <div
                  className={
                    row[0].width === "half"
                      ? "grid grid-cols-1 gap-4 @min-[720px]:grid-cols-2"
                      : "grid grid-cols-1 gap-4"
                  }
                >
                  {row.map((item, column) => (
                    <div key={column} className="min-w-0">
                      <RcaItem token={token} item={item} caption={caption} />
                    </div>
                  ))}
                </div>
              </li>
            ))}
          </ol>
        </main>
      </div>
    </DatasourceListProvider>
  )
}

export function PublicRcaLoadError({
  error,
  retrying,
  onRetry,
}: {
  error: unknown
  retrying: boolean
  onRetry: () => void
}) {
  return (
    <main className="mx-auto w-full max-w-lg p-6">
      <LoadProblem
        title="Could not load this RCA"
        detail={loadProblemText(error)}
        retrying={retrying}
        onRetry={onRetry}
      />
    </main>
  )
}
