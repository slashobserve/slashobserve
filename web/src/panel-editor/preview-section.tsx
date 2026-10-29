import { useQuery } from "@tanstack/react-query"
import { CheckIcon, CircleAlertIcon, LoaderCircleIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import type { Selections } from "@/api/keys"
import { useNow } from "@/components/relative-time"
import { Skeleton } from "@/components/ui/skeleton"
import { maxDataPointsFor, usePanelData } from "@/dashboards/use-panel-data"
import { sameJson, stableJson } from "@/lib/json"
import { describeRange } from "@/lib/time"
import { PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { noSharedCursor } from "@/panels/shared-cursor"
import { prepareFrames } from "@/panels/transforms"
import {
  isDataPanel,
  validateSpecSchema,
  type DataPanel,
  type Dashboard,
} from "@/spec"

import type { EditablePanel } from "./draft"
import { withPanel } from "./panel-json"
import {
  previewErrorText,
  previewStatusText,
  seriesCount,
  type PreviewStatus,
} from "./preview-status"
import { previewQuery } from "./queries"
import { useDebouncedValue } from "./use-debounced-value"

export type PreviewSectionProps = {
  dashboardId: string
  spec: Dashboard
  version: number
  baseVersion: number
  base: EditablePanel
  draft: EditablePanel
  timeRange: { from?: string; to?: string }
  selections: Selections | null
}

const debounceMs = 600
const chartHeight = 200
const noSelections: Selections = {}

function ignoreSelection() {}

function useContentWidth() {
  const ref = useRef<HTMLElement>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) => {
      setWidth(Math.floor(entry.contentRect.width))
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return { ref, width }
}

function draftIsValid(spec: Dashboard, draft: DataPanel): boolean {
  if (draft.queries.some((query) => query.query.trim() === "")) return false
  const placed = withPanel(spec, draft.id, draft)
  return placed !== null && validateSpecSchema(placed).valid
}

function StatusIcon({ kind }: { kind: PreviewStatus["kind"] }) {
  switch (kind) {
    case "ok":
      return <CheckIcon aria-hidden="true" className="size-3 text-status-ok" />
    case "invalid":
    case "error":
      return (
        <CircleAlertIcon
          aria-hidden="true"
          className="size-3 text-status-critical"
        />
      )
    case "running":
    case "waiting":
      return (
        <LoaderCircleIcon
          aria-hidden="true"
          className="size-3 motion-safe:animate-spin"
        />
      )
  }
}

function PreviewHeader({
  status,
  range,
}: {
  status: PreviewStatus
  range: string
}) {
  const now = useNow()
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5 pb-1.5 pl-2 text-xs text-muted-foreground">
      <span
        role="status"
        aria-live="polite"
        className="flex items-center gap-1.5"
      >
        <StatusIcon kind={status.kind} />
        {previewStatusText(status, now)}
      </span>
      <span>{range}</span>
    </div>
  )
}

type DataPreviewProps = Omit<PreviewSectionProps, "draft"> & {
  draft: DataPanel
  width: number
}

function DataPreview({
  dashboardId,
  spec,
  version,
  baseVersion,
  base,
  draft,
  timeRange,
  selections,
  width,
}: DataPreviewProps) {
  const debounced = useDebouncedValue(draft, stableJson(draft), debounceMs)
  const valid = useMemo(() => draftIsValid(spec, debounced), [spec, debounced])
  const vars = selections ?? noSelections
  const maxDataPoints = maxDataPointsFor(debounced, width)
  const ready = selections !== null && width > 0

  const preview = useQuery({
    ...previewQuery({
      dashboardId,
      version,
      panel: debounced,
      from: timeRange.from,
      to: timeRange.to,
      selections: vars,
      maxDataPoints,
    }),
    enabled: valid && ready,
  })

  const showGhost =
    isDataPanel(base) &&
    (!sameJson(base.queries, debounced.queries) ||
      base.datasource !== debounced.datasource)
  const saved = usePanelData({
    dashboardId,
    version: baseVersion,
    panel: base,
    from: timeRange.from,
    to: timeRange.to,
    vars,
    maxDataPoints,
    refreshMs: null,
    enabled: showGhost && ready,
  })

  const data = preview.data
  const status = useMemo<PreviewStatus>(() => {
    if (!valid) return { kind: "invalid" }
    if (selections === null) return { kind: "waiting" }
    if (preview.isError) {
      return { kind: "error", text: previewErrorText(preview.error) }
    }
    if (data === undefined || preview.isPlaceholderData) {
      return { kind: "running" }
    }
    return {
      kind: "ok",
      executedAt: data.meta.executed_at,
      series: seriesCount(prepareFrames(debounced, data.frames)),
    }
  }, [
    valid,
    selections,
    preview.isError,
    preview.error,
    preview.isPlaceholderData,
    data,
    debounced,
  ])

  let body
  if (data) {
    body = (
      <PanelBody
        panel={debounced}
        data={data}
        ghost={showGhost ? saved.data : undefined}
        width={width}
        height={chartHeight}
        stateScope={`preview:${dashboardId}`}
        cursor={noSharedCursor}
        onTimeRangeSelect={ignoreSelection}
        vars={vars}
        from={timeRange.from}
        to={timeRange.to}
      />
    )
  } else if (status.kind === "error") {
    body = (
      <div
        className="flex items-center justify-center px-4 text-center text-[13px] text-status-critical"
        style={{ height: chartHeight }}
      >
        {status.text}
      </div>
    )
  } else if (status.kind === "invalid") {
    body = <div style={{ height: chartHeight }} />
  } else {
    body = <Skeleton style={{ height: chartHeight }} />
  }

  return (
    <>
      <PreviewHeader
        status={status}
        range={describeRange(
          timeRange.from ?? spec.time.from,
          timeRange.to ?? spec.time.to
        )}
      />
      {body}
      {data && showGhost && saved.data ? (
        <div className="flex flex-wrap gap-x-4 gap-y-0.5 pt-2 pl-[52px] text-xs">
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <span
              aria-hidden="true"
              className="w-3.5 border-t-2 border-dotted border-ghost"
            />
            Saved in v{baseVersion}
          </span>
        </div>
      ) : null}
    </>
  )
}

export function PreviewSection({
  dashboardId,
  spec,
  version,
  baseVersion,
  base,
  draft,
  timeRange,
  selections,
}: PreviewSectionProps) {
  const { ref, width } = useContentWidth()
  let content = null
  if (width > 0) {
    content = isDataPanel(draft) ? (
      <DataPreview
        dashboardId={dashboardId}
        spec={spec}
        version={version}
        baseVersion={baseVersion}
        base={base}
        draft={draft}
        timeRange={timeRange}
        selections={selections}
        width={width}
      />
    ) : (
      <>
        <div className="pb-1.5 pl-2 text-xs text-muted-foreground">Preview</div>
        <StaticPanelBody
          panel={draft}
          width={width}
          height={chartHeight}
          vars={selections ?? noSelections}
          from={timeRange.from ?? spec.time.from}
          to={timeRange.to ?? spec.time.to}
        />
      </>
    )
  }
  return (
    <section
      ref={ref}
      aria-label="Preview"
      className="rounded-[8px] border border-border py-2.5 pr-3 pl-1"
    >
      {content}
    </section>
  )
}
