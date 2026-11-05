import { useMemo, useState } from "react"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { StatusIcon } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { asDashboardSpec } from "@/dashboards/queries"
import { cn } from "@/lib/utils"
import {
  buildQuery,
  buildStatusText,
  canContinueBuild,
  newPanelIds,
  saveBuildErrorText,
  useSaveBuild,
  type BuildProgress,
} from "@/observe/build"
import type { ObserveRequest } from "@/observe/queries"
import { ContinueBuildButton } from "@/observe/request-actions"
import { isRowPanel, panelTitle, type Dashboard, type Panel } from "@/spec"
import { dashboardLink } from "@/lib/links"

import { Thumbnail, thumbnailColumns, thumbnailSpan } from "./result-card"

const maxShownRows = 4
const noPanels: ReadonlySet<string> = new Set()
const fadeIn =
  "motion-safe:animate-in motion-safe:fade-in-0 motion-safe:duration-300"

export type BuildRow = { id: string; title: string | null; panels: Panel[] }

export function buildRows(spec: Dashboard): BuildRow[] {
  const rows: BuildRow[] = []
  let loose: BuildRow | undefined
  for (const panel of spec.panels) {
    if (isRowPanel(panel)) {
      loose = undefined
      const children = panel.panels ?? []
      if (children.length > 0) {
        rows.push({ id: panel.id, title: panelTitle(panel), panels: children })
      }
    } else {
      if (loose === undefined) {
        loose = { id: `loose-${panel.id}`, title: null, panels: [] }
        rows.push(loose)
      }
      loose.panels.push(panel)
    }
  }
  return rows
}

export function shownRows(
  rows: readonly BuildRow[],
  max = maxShownRows
): { shown: BuildRow[]; hidden: number } {
  return {
    shown: rows.slice(Math.max(0, rows.length - max)),
    hidden: Math.max(0, rows.length - max),
  }
}

export function latestThumbnails(panels: readonly Panel[]): Panel[] {
  const ordered = [...panels].sort((a, b) =>
    a.gridPos.y === b.gridPos.y
      ? a.gridPos.x - b.gridPos.x
      : a.gridPos.y - b.gridPos.y
  )
  const shown: Panel[] = []
  let used = 0
  for (const panel of ordered.reverse()) {
    used += thumbnailSpan(panel)
    if (used > thumbnailColumns) break
    shown.push(panel)
  }
  return shown.reverse()
}

function useFreshPanels(
  spec: Dashboard | undefined,
  saves: number | undefined
) {
  const [seen, setSeen] = useState<{
    saves: number | undefined
    spec: Dashboard | undefined
    fresh: ReadonlySet<string>
  }>({ saves: undefined, spec: undefined, fresh: noPanels })
  if (spec !== undefined && saves !== seen.saves) {
    setSeen({
      saves,
      spec,
      fresh: seen.spec ? newPanelIds(seen.spec, spec) : noPanels,
    })
  }
  return seen.fresh
}

function Rows({
  rows,
  fresh,
}: {
  rows: readonly BuildRow[]
  fresh: ReadonlySet<string>
}) {
  const { shown, hidden } = shownRows(rows)
  return (
    <div className="flex flex-col gap-2.5 border-t p-2.5">
      {hidden > 0 ? (
        <p className="text-xs text-muted-foreground">
          {hidden} earlier {hidden === 1 ? "row" : "rows"}
        </p>
      ) : null}
      <div aria-hidden="true" className="flex flex-col gap-2.5">
        {shown.map((row) => (
          <div key={row.id} className="flex flex-col gap-1">
            {row.title !== null ? (
              <span className="truncate text-[11px] text-muted-foreground">
                {row.title}
              </span>
            ) : null}
            <div className="grid grid-cols-4 gap-1.5">
              {latestThumbnails(row.panels).map((panel, index) => (
                <div
                  key={panel.id}
                  style={{ gridColumn: `span ${thumbnailSpan(panel)}` }}
                  className={cn("min-w-0", fresh.has(panel.id) && fadeIn)}
                >
                  <Thumbnail panel={panel} index={index} />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

type BuildCardRequest = Pick<
  ObserveRequest,
  | "id"
  | "status"
  | "dashboard_id"
  | "failure_code"
  | "build_saved_at"
  | "continued_by"
> &
  Partial<Pick<ObserveRequest, "queue_position" | "cancel_requested_at">>

function BuildActions({
  request,
  build,
  isRequester,
  onContinued,
}: {
  request: BuildCardRequest
  build: BuildProgress
  isRequester: boolean
  onContinued: (next: ObserveRequest) => void
}) {
  return (
    <div className="flex flex-wrap items-start gap-x-3 gap-y-2 border-t px-3.5 py-2.5 text-[13px]">
      <SaveBuild request={request} />
      <ContinueBuildButton
        request={{ ...request, build }}
        isRequester={isRequester}
        onContinued={onContinued}
      />
    </div>
  )
}

function SaveBuild({
  request,
}: {
  request: Pick<ObserveRequest, "id" | "dashboard_id">
}) {
  const save = useSaveBuild(request.id)
  const busy = save.isPending || save.isSuccess
  const code = save.error instanceof ApiError ? save.error.code : ""
  return (
    <>
      <div className="flex flex-col items-start gap-1">
        <Button
          size="sm"
          disabled={busy}
          aria-busy={busy}
          onClick={() => save.mutate(undefined)}
        >
          {busy ? "Saving" : "Save what was built"}
        </Button>
        {request.dashboard_id === null ? (
          <p className="text-muted-foreground">Saves to your My Dashboards.</p>
        ) : null}
      </div>
      {save.error !== null ? (
        <p role="alert" className="flex flex-wrap items-center gap-x-2">
          <span>{saveBuildErrorText(code, save.error.message)}</span>
          {code === "version_conflict" && request.dashboard_id !== null ? (
            <Link
              {...dashboardLink(request.dashboard_id)}
              className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              Open the dashboard
            </Link>
          ) : null}
        </p>
      ) : null}
    </>
  )
}

type Props = {
  request: BuildCardRequest
  build: BuildProgress
  isRequester: boolean
  onContinued: (next: ObserveRequest) => void
}

export function BuildCard({ request, build, isRequester, onContinued }: Props) {
  const active = request.status === "queued" || request.status === "running"
  const result = useQuery({
    ...buildQuery(request.id, build.saves),
    enabled: isRequester,
    placeholderData: keepPreviousData,
  })
  const loaded = result.data
  const spec = useMemo(
    () => (loaded ? asDashboardSpec(loaded.spec) : undefined),
    [loaded]
  )
  const fresh = useFreshPanels(spec, loaded?.saves)
  const rows = useMemo(() => (spec ? buildRows(spec) : []), [spec])
  const status = buildStatusText({
    status: request.status,
    build,
    failureCode: request.failure_code,
    queuePosition: request.queue_position ?? null,
    canceling: (request.cancel_requested_at ?? null) !== null,
    continued: request.continued_by !== null,
  })
  const mayAct = canContinueBuild({
    status: request.status,
    build,
    buildSavedAt: request.build_saved_at,
    continuedBy: request.continued_by,
    isRequester,
  })

  return (
    <div className="flex flex-col overflow-hidden rounded-card border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-2.5 px-3.5 py-2.5">
        <p
          aria-live="polite"
          className="flex min-w-0 items-center gap-2 text-[13px] wrap-anywhere"
        >
          {active ? <StatusIcon tone="running" /> : null}
          {status}
        </p>
        {isRequester ? (
          <Link
            to="/b/$requestId"
            params={{ requestId: request.id }}
            className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
          >
            Watch live
          </Link>
        ) : null}
      </div>
      {rows.length > 0 ? <Rows rows={rows} fresh={fresh} /> : null}
      {mayAct ? (
        <BuildActions
          request={request}
          build={build}
          isRequester={isRequester}
          onContinued={onContinued}
        />
      ) : null}
    </div>
  )
}
