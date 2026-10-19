import { useQuery } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"

import { useNow } from "@/components/relative-time"
import { StatusIcon } from "@/components/status-label"
import { Skeleton } from "@/components/ui/skeleton"
import { asDashboardSpec, dashboardVersionQuery } from "@/dashboards/queries"
import { panelDatasourceUids } from "@/dashboards/panel-datasources"
import { formatShortRelative } from "@/lib/time"
import { joinNames, lowerFirst } from "@/observe/conversation"
import { allPanels, isRowPanel, panelTitle, type Panel } from "@/spec"

import { OpenDashboardLink } from "./turn-failure"

const thumbnailColumns = 4
const nearViewportMargin = "600px"

const sparklineColors = [
  "stroke-series-1",
  "stroke-series-1",
  "stroke-series-2",
  "stroke-series-3",
]

function sparkline(seed: number, dips: boolean): string {
  return Array.from({ length: 21 }, (_, i) => {
    const wave = Math.sin(i * 0.8 + seed) * 4
    const dip = dips && i > 13 && i < 17 ? -8 : 0
    return `${i * 5},${Math.round(14 + wave + dip)}`
  }).join(" ")
}

function thumbnailSpan(panel: Panel): number {
  return Math.min(
    thumbnailColumns,
    Math.max(1, Math.round(panel.gridPos.w / 6))
  )
}

function thumbnails(panels: Panel[]): Panel[] {
  const ordered = [...panels].sort((a, b) =>
    a.gridPos.y === b.gridPos.y
      ? a.gridPos.x - b.gridPos.x
      : a.gridPos.y - b.gridPos.y
  )
  const shown: Panel[] = []
  let used = 0
  for (const panel of ordered) {
    used += thumbnailSpan(panel)
    if (used > thumbnailColumns) break
    shown.push(panel)
  }
  return shown
}

function Thumbnail({ panel, index }: { panel: Panel; index: number }) {
  return (
    <div
      style={{ gridColumn: `span ${thumbnailSpan(panel)}` }}
      className="flex h-[52px] min-w-0 flex-col justify-between rounded-[5px] border p-1.5"
    >
      <span className="truncate text-[10px] text-muted-foreground">
        {panelTitle(panel)}
      </span>
      <svg
        viewBox="0 0 100 24"
        preserveAspectRatio="none"
        className="h-[22px] w-full"
      >
        <polyline
          points={sparkline(index + 1, index % 2 === 1)}
          fill="none"
          strokeWidth={1.5}
          vectorEffect="non-scaling-stroke"
          className={sparklineColors[index % sparklineColors.length]}
        />
      </svg>
    </div>
  )
}

type Props = {
  dashboardId: string
  version: number
  finishedAt: string | null
  eager: boolean
}

export function ResultCard({ dashboardId, version, finishedAt, eager }: Props) {
  const now = useNow()
  const card = useRef<HTMLDivElement>(null)
  const [nearViewport, setNearViewport] = useState(false)
  const fetchVersion = eager || nearViewport
  const query = useQuery({
    ...dashboardVersionQuery(dashboardId, version),
    enabled: fetchVersion,
  })

  useEffect(() => {
    const element = card.current
    if (
      fetchVersion ||
      element === null ||
      typeof IntersectionObserver === "undefined"
    ) {
      return
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setNearViewport(true)
        }
      },
      { rootMargin: nearViewportMargin }
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [fetchVersion])

  if (query.isLoading) {
    return <Skeleton className="h-[172px] rounded-card" />
  }

  const spec =
    query.data === undefined ? null : asDashboardSpec(query.data.version.spec)
  const panels =
    spec === null
      ? []
      : allPanels(spec)
          .map((entry) => entry.panel)
          .filter((panel) => !isRowPanel(panel))
  const uids = [...new Set(panels.flatMap(panelDatasourceUids))]
  const count = panels.length
  const hint =
    spec === null
      ? "Saved"
      : `${count} ${count === 1 ? "panel" : "panels"}${
          uids.length > 0 ? ` from ${joinNames(uids)}` : ""
        }`
  const shown = thumbnails(panels)

  return (
    <div
      ref={card}
      className="flex flex-col overflow-hidden rounded-card border bg-background"
    >
      {shown.length > 0 ? (
        <div
          aria-hidden="true"
          className="grid grid-cols-4 gap-1.5 border-b p-2.5"
        >
          {shown.map((panel, index) => (
            <Thumbnail key={panel.id} panel={panel} index={index} />
          ))}
        </div>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2.5 px-3.5 py-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="font-semibold wrap-anywhere">
            {spec?.title ?? "Dashboard"}
          </span>
          <span className="text-xs wrap-anywhere text-muted-foreground">
            {hint}
          </span>
        </div>
        <OpenDashboardLink dashboardId={dashboardId} variant="primary" />
      </div>
      <div className="flex items-center gap-1.5 border-t px-3.5 py-[9px] text-xs text-muted-foreground">
        <StatusIcon tone="ok" />
        <span>
          Saved as version {version}
          {finishedAt === null ? null : (
            <>
              {", "}
              <time dateTime={finishedAt}>
                {lowerFirst(formatShortRelative(finishedAt, now))}
              </time>
            </>
          )}
        </span>
      </div>
    </div>
  )
}
