import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { HistoryIcon, MessageSquareIcon } from "lucide-react"

import type { Selections } from "@/api/keys"
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { useComments } from "@/comments/queries"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { FollowUpBar } from "@/revisions/follow-up-bar"
import { useRevisionAppliedToast } from "@/revisions/use-revision-applied-toast"
import { refreshToMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import { panelRenderers } from "@/panels"
import { hasData } from "@/panels/frames"
import type { Panel } from "@/spec"

import { DashboardGrid, gridMargin, gridRowHeight } from "./dashboard-grid"
import { DashboardToolbar } from "./dashboard-toolbar"
import { EditControls, NewerVersionNotice } from "./edit/edit-controls"
import { useGridEditable, useLayoutStore } from "./edit/layout-store"
import { LayoutAnnouncer } from "./edit/panel-move-handle"
import { UnsavedChangesGuard } from "./edit/unsaved-changes-guard"
import { LiveIndicator } from "./live/live-indicator"
import {
  useDashboardEvents,
  type LiveStatus,
} from "./live/use-dashboard-events"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { PanelInspector } from "./panel-inspector"
import { PanelMenu } from "./panel-menu"
import {
  asDashboardSpec,
  dashboardVersionQuery,
  latestDashboardQuery,
  type DashboardEnvelope,
} from "./queries"
import { RefreshPicker } from "./refresh-picker"
import {
  backFromSearch,
  useDashboardSearch,
  useSetDashboardSearch,
} from "./search"
import { TimeRangePicker } from "./time-range-picker"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { useSeen } from "./use-seen"
import { useVariables } from "./use-variables"
import { VariablesBar } from "./variables-bar"
import { VersionBanner } from "./version-banner"
import { VariableLoadError } from "./variables"

type PanelContainerProps = {
  dashboardId: string
  version: number
  panel: Panel
  width: number
  height: number
  from: string | undefined
  to: string | undefined
  selections: Selections
  selectionsReady: boolean
  blockedBy: string | undefined
  onRetryBlocked: () => void
  refreshMs: number | null
  editable: boolean
  layoutSession: boolean
  openComments: number
}

const PanelContainer = memo(function PanelContainer({
  dashboardId,
  version,
  panel,
  width,
  height,
  from,
  to,
  selections,
  selectionsReady,
  blockedBy,
  onRetryBlocked,
  refreshMs,
  editable,
  layoutSession,
  openComments,
}: PanelContainerProps) {
  const ref = useRef<HTMLDivElement>(null)
  const seen = useSeen(ref)
  const [tableView, setTableView] = useState(false)
  const setSearch = useSetDashboardSearch()
  const body = panelBodySize(Math.floor(width), Math.floor(height))

  const query = usePanelData({
    dashboardId,
    version,
    panel,
    from,
    to,
    vars: selections,
    maxDataPoints: maxDataPointsFor(panel, body.width),
    refreshMs,
    enabled: seen && selectionsReady,
  })

  const onTimeRangeSelect = useCallback(
    (fromMs: number, toMs: number) =>
      setSearch((state) => ({
        ...state,
        from: String(Math.round(fromMs)),
        to: String(Math.round(toMs)),
      })),
    [setSearch]
  )

  let state: PanelState = "loading"
  if (query.data) state = hasData(query.data.frames) ? "ready" : "empty"
  else if (blockedBy !== undefined) state = "error"
  else if (query.isError) state = "error"

  const blockedError = useMemo(
    () =>
      blockedBy === undefined ? undefined : new VariableLoadError(blockedBy),
    [blockedBy]
  )

  const Renderer = panelRenderers[tableView ? "table" : panel.type]

  return (
    <div ref={ref} className="size-full">
      <PanelChrome
        panel={panel}
        state={state}
        error={query.data ? query.error : (blockedError ?? query.error)}
        refreshing={query.isFetching && query.data !== undefined}
        staleError={query.isError && query.data ? query.error : undefined}
        onRetry={() => (blockedError ? onRetryBlocked() : void query.refetch())}
        dragHandle={editable}
        badge={
          <PanelCommentBadge
            panelId={panel.id}
            title={panel.title}
            count={openComments}
            disabled={layoutSession}
          />
        }
        menu={
          <PanelMenu
            panel={panel}
            tableView={tableView}
            onToggleTableView={() => setTableView((current) => !current)}
            inspectable={!layoutSession}
          />
        }
      >
        {query.data ? (
          <Renderer
            panel={panel}
            frames={query.data.frames}
            meta={query.data.meta}
            width={body.width}
            height={body.height}
            syncKey={dashboardId}
            onTimeRangeSelect={onTimeRangeSelect}
          />
        ) : null}
      </PanelChrome>
    </div>
  )
})

function DashboardView({
  dashboardId,
  envelope,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  envelope: DashboardEnvelope
  latestVersion: number
  liveStatus: LiveStatus
}) {
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const { comments } = useComments(dashboardId)
  const openByPanel = useMemo(() => {
    const counts = new Map<string, number>()
    for (const comment of comments) {
      if (comment.status !== "open") continue
      counts.set(comment.panel_id, (counts.get(comment.panel_id) ?? 0) + 1)
    }
    return counts
  }, [comments])
  const openTotal = useMemo(
    () => comments.reduce((n, c) => n + (c.status === "open" ? 1 : 0), 0),
    [comments]
  )
  const envelopeVersion = envelope.version.version
  const envelopeSpec = useMemo(
    () => asDashboardSpec(envelope.version.spec),
    [envelope.version.spec]
  )
  const session = useLayoutStore((state) => state.session)
  const setLayout = useLayoutStore((state) => state.setLayout)
  const gridBox = useRef<HTMLDivElement>(null)
  const canEdit = useGridEditable(gridBox)
  const editing =
    session !== null &&
    session.dashboardId === dashboardId &&
    search.version === undefined
  const version = editing ? session.baseVersion : envelopeVersion
  const spec = editing ? session.baseSpec : envelopeSpec
  const variables = useMemo(() => spec.variables ?? [], [spec])

  const { states, selections, ready, blocking } = useVariables({
    dashboardId,
    version,
    variables,
    from: search.from,
    to: search.to,
    urlVars: search.vars,
  })

  const retryBlocking = useRef<(() => void) | undefined>(undefined)
  useEffect(() => {
    retryBlocking.current = blocking?.retry
  })
  const onRetryBlocked = useCallback(() => retryBlocking.current?.(), [])
  const blockedBy = blocking
    ? (blocking.variable.label ?? blocking.variable.name)
    : undefined

  useEffect(() => {
    const previous = document.title
    document.title = `${spec.title} - SlashObserve`
    return () => {
      document.title = previous
    }
  }, [spec.title])

  const effectiveFrom = search.from ?? spec.timeRange.from
  const effectiveTo = search.to ?? spec.timeRange.to
  const refreshValue = search.refresh ?? spec.refresh ?? "off"
  const refreshMs = refreshToMs(refreshValue)

  const controls = [
    <TimeRangePicker
      key="time-range"
      dashboardId={dashboardId}
      version={version}
      from={effectiveFrom}
      to={effectiveTo}
    />,
    <RefreshPicker key="refresh" value={refreshValue} />,
  ]
  const actions: ReactNode[] = [
    <LiveIndicator
      key="live"
      status={liveStatus}
      pinned={search.version !== undefined}
    />,
    <Button
      key="comments"
      variant="outline"
      data-comments-trigger=""
      disabled={editing}
      aria-label={openTotal > 0 ? `Comments, ${openTotal} open` : "Comments"}
      onClick={() =>
        setSearch((state) => ({ ...state, panel: undefined, tab: "comments" }))
      }
    >
      <MessageSquareIcon data-icon="inline-start" />
      Comments
      {openTotal > 0 ? (
        <Badge variant="secondary" className="tabular-nums">
          {openTotal}
        </Badge>
      ) : null}
    </Button>,
    <Link
      key="history"
      to="/dashboards/$dashboardId/versions"
      params={{ dashboardId }}
      search={{ back: backFromSearch(search) }}
      className={cn(buttonVariants({ variant: "outline" }))}
    >
      <HistoryIcon data-icon="inline-start" />
      History
    </Link>,
    <EditControls
      key="edit-layout"
      dashboardId={dashboardId}
      version={envelopeVersion}
      spec={envelopeSpec}
      pinned={search.version !== undefined}
      canEdit={canEdit}
    />,
  ]

  return (
    <main className="flex flex-1 flex-col">
      <DashboardToolbar
        title={spec.title}
        versionBadge={`v${version}`}
        controls={controls}
        actions={actions}
      />
      {search.version !== undefined ? (
        <VersionBanner version={version} latestVersion={latestVersion} />
      ) : null}
      {editing ? (
        <NewerVersionNotice
          dashboardId={dashboardId}
          baseVersion={session.baseVersion}
          latestVersion={latestVersion}
        />
      ) : null}
      <VariablesBar states={states} />
      <FollowUpBar
        dashboardId={dashboardId}
        version={version}
        panels={spec.panels}
        canAsk={search.version === undefined && !editing}
      />
      <LayoutAnnouncer />
      <div ref={gridBox} className="flex-1 p-4">
        <DashboardGrid
          spec={spec}
          editable={editing && canEdit}
          layout={editing ? session.layout : undefined}
          onLayoutChange={editing ? setLayout : undefined}
          renderPanel={(panel, size) => (
            <PanelContainer
              dashboardId={dashboardId}
              version={version}
              panel={panel}
              width={size.width}
              height={size.height}
              from={search.from}
              to={search.to}
              selections={selections}
              selectionsReady={ready}
              blockedBy={blockedBy}
              onRetryBlocked={onRetryBlocked}
              refreshMs={refreshMs}
              editable={editing && canEdit}
              layoutSession={editing}
              openComments={openByPanel.get(panel.id) ?? 0}
            />
          )}
        />
      </div>
      <PanelInspector
        spec={spec}
        dashboardId={dashboardId}
        version={version}
        latestVersion={latestVersion}
        pinned={search.version !== undefined}
        suspended={editing}
      />
    </main>
  )
}

function PinnedDashboardPage({
  dashboardId,
  version,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  version: number
  latestVersion: number | undefined
  liveStatus: LiveStatus
}) {
  const pinned = useQuery(dashboardVersionQuery(dashboardId, version))
  if (!pinned.data) return <DashboardPageSkeleton />
  return (
    <DashboardView
      dashboardId={dashboardId}
      envelope={pinned.data}
      latestVersion={latestVersion ?? pinned.data.version.version}
      liveStatus={liveStatus}
    />
  )
}

export function DashboardPage({ dashboardId }: { dashboardId: string }) {
  const search = useDashboardSearch()
  const queryClient = useQueryClient()
  const latest = useQuery(latestDashboardQuery(queryClient, dashboardId))
  const latestVersion = latest.data?.dashboard.latest_version
  const editing = useLayoutStore(
    (state) => state.session?.dashboardId === dashboardId
  )
  const { status: liveStatus } = useDashboardEvents(dashboardId, {
    pinnedVersion: search.version,
    toastVersions: !editing,
  })
  useRevisionAppliedToast(dashboardId, !editing)
  useEffect(() => () => useLayoutStore.getState().end(), [dashboardId])
  let content: ReactNode
  if (search.version !== undefined) {
    content = (
      <PinnedDashboardPage
        dashboardId={dashboardId}
        version={search.version}
        latestVersion={latestVersion}
        liveStatus={liveStatus}
      />
    )
  } else if (!latest.data) {
    content = <DashboardPageSkeleton />
  } else {
    content = (
      <DashboardView
        dashboardId={dashboardId}
        envelope={latest.data}
        latestVersion={latestVersion ?? latest.data.version.version}
        liveStatus={liveStatus}
      />
    )
  }
  return (
    <>
      <UnsavedChangesGuard />
      {content}
    </>
  )
}

const skeletonPanels = [
  "min-[800px]:col-span-12 row-span-8",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-24 row-span-8",
]

export function DashboardPageSkeleton() {
  return (
    <main className="flex flex-1 flex-col" aria-busy="true">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b px-4 py-2">
        <Skeleton className="h-7 w-56 animate-none motion-safe:animate-pulse" />
        <Skeleton className="h-8 w-48 animate-none motion-safe:animate-pulse" />
      </div>
      <div className="flex-1 p-4">
        <div
          className="grid grid-cols-1 min-[800px]:grid-cols-24"
          style={{ gridAutoRows: gridRowHeight, gap: gridMargin }}
        >
          {skeletonPanels.map((span, index) => (
            <Skeleton
              key={index}
              className={`${span} animate-none motion-safe:animate-pulse`}
            />
          ))}
        </div>
      </div>
    </main>
  )
}
