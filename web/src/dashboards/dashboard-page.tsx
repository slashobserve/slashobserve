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

import type { Selections } from "@/api/keys"
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { panelRefs } from "@/comments/latest-panels"
import { useComments } from "@/comments/queries"
import { openThreadCount, openThreadsByPanel } from "@/comments/threads"
import { Skeleton } from "@/components/ui/skeleton"
import { FollowUpBar } from "@/revisions/follow-up-bar"
import { useRevisionAppliedToast } from "@/revisions/use-revision-applied-toast"
import { refreshToMs } from "@/lib/time"
import { madeByBreadcrumb } from "@/lib/origin"
import { panelRenderers } from "@/panels"
import { hasData } from "@/panels/frames"
import { ThemeSwitch } from "@/shell/theme-switch"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { ActivityToggle } from "./activity-toggle"
import { DashboardGrid, gridMargin, gridRowHeight } from "./dashboard-grid"
import { DashboardTopRow } from "./dashboard-top-row"
import { EditControls, NewerVersionNotice } from "./edit/edit-controls"
import { useGridEditable, useLayoutStore } from "./edit/layout-store"
import { LayoutAnnouncer } from "./edit/panel-move-handle"
import { UnsavedChangesGuard } from "./edit/unsaved-changes-guard"
import {
  useDashboardEvents,
  type LiveStatus,
} from "./live/use-dashboard-events"
import { downloadSpec } from "./export-spec"
import { DashboardLoadError } from "./load-error"
import { MoreMenu } from "./more-menu"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { panelDatasourceUids } from "./panel-datasources"
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
import { VersionMenu } from "./version-menu"
import { controlVariables, VariableLoadError } from "./variables"

type PanelContainerProps = {
  dashboardId: string
  version: number
  panel: LayoutPanel
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
  const supported =
    isDataPanel(panel) && panelRenderers[panel.type] !== undefined

  const query = usePanelData({
    dashboardId,
    version,
    panel,
    from,
    to,
    vars: selections,
    maxDataPoints: maxDataPointsFor(panel, body.width),
    refreshMs,
    enabled: supported && seen && selectionsReady,
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
  if (!supported) state = "unsupported"
  else if (query.data) state = hasData(query.data.frames) ? "ready" : "empty"
  else if (blockedBy !== undefined) state = "error"
  else if (query.isError) state = "error"

  const blockedError = useMemo(
    () =>
      blockedBy === undefined ? undefined : new VariableLoadError(blockedBy),
    [blockedBy]
  )

  const Renderer = isDataPanel(panel)
    ? panelRenderers[tableView ? "table" : panel.type]
    : undefined

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
        datasourceLabel={panelDatasourceUids(panel).join(", ")}
        badge={
          <PanelCommentBadge
            panelId={panel.id}
            title={panelTitle(panel)}
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
        {query.data && Renderer && isDataPanel(panel) ? (
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
  const openByPanel = useMemo(() => openThreadsByPanel(comments), [comments])
  const openTotal = useMemo(() => openThreadCount(comments), [comments])
  const envelopeVersion = envelope.version.version
  const envelopeSpec = useMemo(
    () => asDashboardSpec(envelope.version.spec),
    [envelope.version.spec]
  )
  const session = useLayoutStore((state) => state.session)
  const setLayout = useLayoutStore((state) => state.setLayout)
  const startLayout = useLayoutStore((state) => state.start)
  const gridBox = useRef<HTMLDivElement>(null)
  const canEdit = useGridEditable(gridBox)
  const editing =
    session !== null &&
    session.dashboardId === dashboardId &&
    search.version === undefined
  const version = editing ? session.baseVersion : envelopeVersion
  const spec = editing ? session.baseSpec : envelopeSpec
  const variables = useMemo(
    () => controlVariables(spec.variables ?? []),
    [spec]
  )

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

  const refs = useMemo(() => panelRefs(spec), [spec])
  const effectiveFrom = search.from ?? spec.time.from
  const effectiveTo = search.to ?? spec.time.to
  const refreshValue = search.refresh ?? spec.refresh ?? "off"
  const refreshMs = refreshToMs(refreshValue)

  const pinned = search.version !== undefined
  const activityOpen = search.tab === "comments" && search.panel === undefined

  const controls = [
    <div key="time" className="flex items-center gap-1.5">
      <TimeRangePicker
        dashboardId={dashboardId}
        version={version}
        from={effectiveFrom}
        to={effectiveTo}
      />
      <RefreshPicker value={refreshValue} />
    </div>,
  ]
  const actions: ReactNode[] = [
    <ThemeSwitch key="theme" />,
    <MoreMenu
      key="more"
      canEditLayout={canEdit && !editing}
      pinned={pinned}
      onEditLayout={() =>
        startLayout(dashboardId, envelopeVersion, envelopeSpec)
      }
      onExport={() => downloadSpec(spec, spec.title, version)}
    />,
    <ActivityToggle
      key="activity"
      pressed={activityOpen}
      openCount={openTotal}
      disabled={editing}
      onToggle={() =>
        setSearch((state) => ({
          ...state,
          panel: undefined,
          tab: activityOpen ? undefined : "comments",
        }))
      }
    />,
  ]

  return (
    <div className="flex h-svh flex-wrap bg-background text-foreground narrow:h-auto narrow:min-h-svh">
      <div className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
        <DashboardTopRow
          title={spec.title}
          madeBy={madeByBreadcrumb(envelope.dashboard.creator)}
          versionControl={
            <VersionMenu
              dashboardId={dashboardId}
              version={version}
              latest={latestVersion}
              current={envelope.version}
              pinned={pinned}
              editing={editing}
              status={liveStatus}
              back={backFromSearch(search)}
            />
          }
          controls={controls}
          editing={<EditControls dashboardId={dashboardId} pinned={pinned} />}
          actions={actions}
        />
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
          panels={refs}
          canAsk={!pinned && !editing}
        />
        <LayoutAnnouncer />
        <main
          ref={gridBox}
          className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4"
        >
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
        </main>
      </div>
      <PanelInspector
        spec={spec}
        dashboardId={dashboardId}
        version={version}
        latestVersion={latestVersion}
        pinned={pinned}
        suspended={editing}
      />
    </div>
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
  if (!pinned.data) {
    if (!pinned.isError) return <DashboardPageSkeleton />
    return (
      <DashboardLoadError
        title={`Could not load version ${version}`}
        error={pinned.error}
        retrying={pinned.isFetching}
        onRetry={() => void pinned.refetch()}
      />
    )
  }
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
    <div
      className="flex h-svh flex-wrap bg-background text-foreground narrow:h-auto narrow:min-h-svh"
      aria-busy="true"
    >
      <div className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 pt-3 pr-2.5 pb-2.5 pl-4">
          <Skeleton className="h-[30px] w-64 animate-none motion-safe:animate-pulse" />
          <Skeleton className="h-[30px] w-48 animate-none motion-safe:animate-pulse" />
        </div>
        <div className="pr-2.5 pb-2.5 pl-4">
          <Skeleton className="h-7 w-48 animate-none motion-safe:animate-pulse" />
        </div>
        <main className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4">
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
        </main>
      </div>
    </div>
  )
}
