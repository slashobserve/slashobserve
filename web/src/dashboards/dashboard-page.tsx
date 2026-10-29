import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type RefObject,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import type { Selections } from "@/api/keys"
import { ActivityCard, activityCardId } from "@/activity/activity-card"
import type { EditGuard } from "@/activity/edit-pane"
import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { useComments } from "@/comments/queries"
import { openThreadCount, openThreadsByPanel } from "@/comments/threads"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { refreshToMs } from "@/lib/time"
import { madeByBreadcrumb } from "@/lib/origin"
import { isRenderable, isStaticPanel, preloadRenderer } from "@/panels"
import { createFieldResolver, panelLinks } from "@/panels/field-config"
import { linkScopeFor, PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import {
  createSharedCursor,
  type CursorSync,
  type SharedCursor,
} from "@/panels/shared-cursor"
import { ThemeSwitch } from "@/shell/theme-switch"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { ActivityToggle } from "./activity-toggle"
import {
  cursorOverride,
  cursorSyncLabel,
  nextCursorSync,
  savedCursorSync,
  useCursorSyncShortcut,
} from "./cursor-sync"
import { DashboardGrid, gridMargin, gridRowHeight } from "./dashboard-grid"
import { DashboardLinks } from "./dashboard-links"
import { DashboardTopRow } from "./dashboard-top-row"
import {
  EditControls,
  findFirstMoveHandle,
  NewerVersionNotice,
} from "./edit/edit-controls"
import {
  focusWhenIdle,
  useGridEditable,
  useLayoutStore,
} from "./edit/layout-store"
import { LayoutAnnouncer } from "./edit/panel-move-handle"
import { UnsavedChangesGuard } from "./edit/unsaved-changes-guard"
import {
  useDashboardEvents,
  type LiveStatus,
} from "./live/use-dashboard-events"
import { downloadSpec } from "./export-spec"
import { MoreMenu } from "./more-menu"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { panelDatasourceUids } from "./panel-datasources"
import { PanelMenu } from "./panel-menu"
import type { DashboardAccess } from "./permissions"
import {
  asDashboardSpec,
  dashboardVersionQuery,
  latestDashboardQuery,
  type DashboardEnvelope,
} from "./queries"
import { RefreshPicker } from "./refresh-picker"
import {
  backFromSearch,
  panelEditorShowsNewerVersion,
  useDashboardSearch,
  useSetDashboardSearch,
} from "./search"
import { TimeRangePicker } from "./time-range-picker"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { useSeen } from "./use-seen"
import { UnverifiedLabel } from "./unverified-label"
import { unverifiedReasons } from "./unverified"
import { useVariables } from "./use-variables"
import { VariablesBar } from "./variables-bar"
import { VersionMenu } from "./version-menu"
import { VariableLoadError } from "./variables"

function pageClass(stacked: boolean) {
  return cn(
    "flex bg-background text-foreground",
    stacked
      ? "min-h-svh flex-wrap"
      : "h-svh flex-nowrap narrow:h-auto narrow:min-h-svh"
  )
}

type PanelContainerProps = {
  dashboardId: string
  cursor: SharedCursor
  version: number
  panel: LayoutPanel
  title: string
  repeatOf: string | undefined
  width: number
  height: number
  from: string | undefined
  to: string | undefined
  rangeFrom: string
  rangeTo: string
  selections: Selections
  selectionsReady: boolean
  blockedBy: string | undefined
  onRetryBlocked: () => void
  refreshMs: number | null
  editable: boolean
  layoutSession: boolean
  openComments: number
  unverifiedReason: string | undefined
  highlighted: boolean
  editingPanel: boolean
  canComment: boolean
  canEdit: boolean
  composerRef: RefObject<HTMLTextAreaElement | null>
}

const PanelContainer = memo(function PanelContainer({
  dashboardId,
  cursor,
  version,
  panel: sourcePanel,
  title,
  repeatOf,
  width,
  height,
  from,
  to,
  rangeFrom,
  rangeTo,
  selections,
  selectionsReady,
  blockedBy,
  onRetryBlocked,
  refreshMs,
  editable,
  layoutSession,
  openComments,
  unverifiedReason,
  highlighted,
  editingPanel,
  canComment,
  canEdit,
  composerRef,
}: PanelContainerProps) {
  const panel = useMemo(
    () =>
      title === panelTitle(sourcePanel)
        ? sourcePanel
        : { ...sourcePanel, title },
    [sourcePanel, title]
  )
  const ref = useRef<HTMLDivElement>(null)
  const seen = useSeen(ref)
  const [tableView, setTableView] = useState(false)
  const setSearch = useSetDashboardSearch()
  const body = panelBodySize(Math.floor(width), Math.floor(height))
  const dataPanel = isDataPanel(panel) ? panel : undefined
  const supported = isRenderable(panel.type)

  useEffect(() => {
    if (seen) preloadRenderer(panel.type)
  }, [seen, panel.type])

  const query = usePanelData({
    dashboardId,
    version,
    panel,
    from,
    to,
    vars: selections,
    maxDataPoints: maxDataPointsFor(panel, body.width),
    refreshMs,
    enabled: dataPanel !== undefined && supported && seen && selectionsReady,
  })

  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel) : undefined),
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
  const links = useMemo(
    () =>
      dataPanel && fields
        ? panelLinks(
            dataPanel,
            linkScopeFor({
              vars: selections,
              from: rangeFrom,
              to: rangeTo,
              fields,
              meta: query.data?.meta,
            })
          )
        : [],
    [dataPanel, fields, selections, rangeFrom, rangeTo, query.data?.meta]
  )

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
  else if (!dataPanel) state = "ready"
  else if (query.data) state = populated ? "ready" : "empty"
  else if (blockedBy !== undefined) state = "error"
  else if (query.isError) state = "error"

  const blockedError = useMemo(
    () =>
      blockedBy === undefined ? undefined : new VariableLoadError(blockedBy),
    [blockedBy]
  )

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
        highlighted={highlighted}
        editing={editingPanel}
        repeatOf={repeatOf}
        datasourceLabel={panelDatasourceUids(panel, selections).join(", ")}
        links={links}
        badge={
          <>
            {unverifiedReason ? (
              <UnverifiedLabel
                title={panelTitle(panel)}
                reason={unverifiedReason}
              />
            ) : null}
            <PanelCommentBadge
              panelId={panel.id}
              title={panelTitle(panel)}
              count={openComments}
              disabled={layoutSession}
            />
          </>
        }
        menu={
          <PanelMenu
            panel={panel}
            tableView={tableView}
            onToggleTableView={() => setTableView((current) => !current)}
            inspectable={!layoutSession}
            canComment={canComment}
            canEdit={canEdit}
            composerRef={composerRef}
          />
        }
      >
        {dataPanel && query.data ? (
          <PanelBody
            panel={dataPanel}
            data={query.data}
            width={body.width}
            height={body.height}
            stateScope={dashboardId}
            cursor={cursor}
            onTimeRangeSelect={onTimeRangeSelect}
            vars={selections}
            from={rangeFrom}
            to={rangeTo}
            asTable={tableView}
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

function DashboardView({
  dashboardId,
  envelope,
  access,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  envelope: DashboardEnvelope
  access: DashboardAccess
  latestVersion: number
  liveStatus: LiveStatus
}) {
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const { permissions } = access
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
  const unverified = useMemo(
    () =>
      unverifiedReasons(envelope.version.unverified, envelopeVersion, version),
    [envelope.version.unverified, envelopeVersion, version]
  )
  const variables = useMemo(() => spec.variables ?? [], [spec])

  const { states, selections, values, ready, blocking } = useVariables({
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

  const effectiveFrom = search.from ?? spec.time.from
  const effectiveTo = search.to ?? spec.time.to
  const timeRange = useMemo(
    () => ({ from: search.from, to: search.to }),
    [search.from, search.to]
  )
  const refreshValue = search.refresh ?? spec.refresh ?? "off"
  const refreshMs = refreshToMs(refreshValue)

  const pinned = search.version !== undefined
  const open = useActivityStore((state) => state.open)
  const setOpen = useActivityStore((state) => state.setOpen)
  const panelInUrl = search.edit !== undefined && search.panel !== undefined
  const editingPanel = permissions.edit && panelInUrl
  const cardVisible = (open || panelInUrl) && !editing
  const cardLayout = useActivityLayout(editingPanel)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const editGuard = useRef<EditGuard>(null)
  const [cursor] = useState(createSharedCursor)
  const savedCursor = savedCursorSync(spec)
  const cursorSync = search.cursor ?? savedCursor
  useLayoutEffect(() => cursor.setMode(cursorSync), [cursor, cursorSync])
  const chooseCursorSync = useCallback(
    (next: CursorSync) =>
      setSearch(
        (state) => ({ ...state, cursor: cursorOverride(next, savedCursor) }),
        { replace: true }
      ),
    [setSearch, savedCursor]
  )
  useCursorSyncShortcut(() => {
    const next = nextCursorSync(cursorSync)
    chooseCursorSync(next)
    toast(`Chart hover: ${cursorSyncLabel(next)}`)
  })
  const [flashed, setFlashed] = useState<string | null>(null)
  const flashTimer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(flashTimer.current), [])

  const layoutEditingWithPanel =
    editing && (search.panel !== undefined || search.edit !== undefined)
  useEffect(() => {
    if (!layoutEditingWithPanel) return
    setSearch((state) => ({ ...state, panel: undefined, edit: undefined }), {
      replace: true,
    })
  }, [layoutEditingWithPanel, setSearch])

  const closeCard = useCallback(() => {
    setOpen(false)
    if (search.panel === undefined && search.edit === undefined) return
    setSearch((state) => ({ ...state, panel: undefined, edit: undefined }))
  }, [setOpen, setSearch, search.panel, search.edit])

  const toggleCard = useCallback(() => {
    if (cardVisible) {
      closeCard()
      return
    }
    setOpen(true)
    if (cardLayout.stacked) {
      requestAnimationFrame(() =>
        document.getElementById(activityCardId)?.scrollIntoView({
          block: "nearest",
          behavior: window.matchMedia("(prefers-reduced-motion: reduce)")
            .matches
            ? "auto"
            : "smooth",
        })
      )
    }
  }, [cardVisible, closeCard, setOpen, cardLayout.stacked])

  const onSelectPanel = useCallback((id: string) => {
    document
      .querySelector(`[data-panel-id="${CSS.escape(id)}"]`)
      ?.scrollIntoView({
        block: "nearest",
        behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "auto"
          : "smooth",
      })
    setFlashed(id)
    window.clearTimeout(flashTimer.current)
    flashTimer.current = window.setTimeout(() => setFlashed(null), 2000)
  }, [])

  const beginLayoutEdit = () => {
    const start = () => {
      startLayout(dashboardId, envelopeVersion, envelopeSpec)
      focusWhenIdle(findFirstMoveHandle)
    }
    if (editGuard.current) editGuard.current.leaveThen(start)
    else start()
  }

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
      mayEdit={permissions.edit}
      canEditLayout={canEdit && !editing}
      layoutBlockedByActivity={
        !canEdit && !editing && cardVisible && !cardLayout.stacked
      }
      pinned={pinned}
      onEditLayout={beginLayoutEdit}
      onExport={() => downloadSpec(spec, spec.title, version)}
      cursorSync={cursorSync}
      onCursorSyncChange={chooseCursorSync}
    />,
    <ActivityToggle
      key="activity"
      pressed={cardVisible}
      openCount={openTotal}
      disabled={editing}
      onToggle={toggleCard}
    />,
  ]

  return (
    <div className={pageClass(cardVisible && cardLayout.stacked)}>
      <div className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
        <DashboardTopRow
          title={spec.title}
          folder={access.folder}
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
        <VariablesBar states={states} unverified={unverified.variables} />
        <DashboardLinks
          links={spec.links}
          scope={{ vars: selections, from: effectiveFrom, to: effectiveTo }}
        />
        <LayoutAnnouncer />
        <main
          ref={gridBox}
          className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4"
        >
          <DashboardGrid
            dashboardId={dashboardId}
            spec={spec}
            selections={selections}
            values={values}
            editable={editing && canEdit}
            layout={editing ? session.layout : undefined}
            onLayoutChange={editing ? setLayout : undefined}
            renderPanel={(item, size) => (
              <PanelContainer
                dashboardId={dashboardId}
                cursor={cursor}
                version={version}
                panel={item.panel}
                title={item.title}
                repeatOf={
                  item.key === item.panel.id ? undefined : item.panel.id
                }
                width={size.width}
                height={size.height}
                from={search.from}
                to={search.to}
                rangeFrom={effectiveFrom}
                rangeTo={effectiveTo}
                selections={item.selections}
                selectionsReady={ready}
                blockedBy={blockedBy}
                onRetryBlocked={onRetryBlocked}
                refreshMs={refreshMs}
                editable={editing && canEdit}
                layoutSession={editing}
                openComments={
                  item.first ? (openByPanel.get(item.panel.id) ?? 0) : 0
                }
                unverifiedReason={unverified.panels.get(item.panel.id)}
                highlighted={
                  item.panel.id === search.panel || item.panel.id === flashed
                }
                editingPanel={
                  editingPanel && item.first && item.panel.id === search.panel
                }
                canComment={permissions.comment}
                canEdit={permissions.edit}
                composerRef={composerRef}
              />
            )}
          />
        </main>
      </div>
      {cardVisible ? (
        <ActivityCard
          dashboardId={dashboardId}
          apiId={envelope.dashboard.id}
          spec={spec}
          version={version}
          latestVersion={latestVersion}
          pinned={pinned}
          canEdit={permissions.edit}
          madeBy={madeByBreadcrumb(envelope.dashboard.creator)}
          back={backFromSearch(search)}
          timeRange={timeRange}
          selections={ready ? selections : null}
          composerRef={composerRef}
          guardRef={editGuard}
          onSelectPanel={onSelectPanel}
          onClose={closeCard}
        />
      ) : null}
    </div>
  )
}

function PinnedDashboardPage({
  dashboardId,
  version,
  latestAccess,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  version: number
  latestAccess: DashboardAccess | undefined
  latestVersion: number | undefined
  liveStatus: LiveStatus
}) {
  const pinned = useQuery(dashboardVersionQuery(dashboardId, version))
  if (!pinned.data) return <DashboardPageSkeleton />
  return (
    <DashboardView
      dashboardId={dashboardId}
      envelope={pinned.data}
      access={latestAccess ?? pinned.data.dashboard}
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
    toastVersions: !editing && !panelEditorShowsNewerVersion(search),
  })
  useEffect(() => () => useLayoutStore.getState().end(), [dashboardId])
  let content: ReactNode
  if (search.version !== undefined) {
    content = (
      <PinnedDashboardPage
        dashboardId={dashboardId}
        version={search.version}
        latestAccess={latest.data?.dashboard}
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
        access={latest.data.dashboard}
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
  const open = useActivityStore((state) => state.open)
  const layout = useActivityLayout(false)
  return (
    <div className={pageClass(open && layout.stacked)} aria-busy="true">
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
      {open ? (
        <div
          aria-hidden="true"
          style={{ "--activity-w": `${layout.width}px` } as CSSProperties}
          className={cn(
            "rounded-card border",
            layout.stacked
              ? "mr-2.5 mb-2.5 ml-4 h-[70svh] flex-[1_1_100%]"
              : "m-2 ml-0.5 flex-[0_0_var(--activity-w)]"
          )}
        />
      ) : null}
    </div>
  )
}
