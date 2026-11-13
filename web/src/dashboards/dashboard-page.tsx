import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { ArrowLeftIcon } from "lucide-react"
import { toast } from "sonner"

import { ActivityCard, activityCardId } from "@/activity/activity-card"
import type { EditGuard } from "@/activity/edit-pane"
import {
  useActivityLayout,
  useActivityStore,
  useChatPaneLayout,
} from "@/activity/activity-store"
import { dashboardKeys } from "@/api/keys"
import { ChatPane } from "@/chat/chat-pane"
import { useComments } from "@/comments/queries"
import { openThreadCount, openThreadsByPanel } from "@/comments/threads"
import { Button } from "@/components/ui/button"
import { foldersQuery } from "@/folders/queries"
import { DashboardHealthButton } from "@/health/dashboard-health-button"
import { usePanelHealth } from "@/health/use-dashboard-health"
import { StarToggle } from "@/home/star-toggle"
import {
  InspectPane,
  inspectPaneId,
  type InspectTarget,
} from "@/inspect/inspect-pane"
import { displayZone, refreshToMs, timeOptionsFor } from "@/lib/time"
import { shift, zoomOut } from "@/lib/time-shift"
import { madeByBreadcrumb } from "@/lib/origin"
import { cn } from "@/lib/utils"
import { isEditablePanel } from "@/panel-editor/draft"
import { isRenderable } from "@/panels"
import { ActionProvider } from "@/panels/actions/action-context"
import { createSharedCursor, type CursorSync } from "@/panels/shared-cursor"
import { ShareToggle } from "@/share/share-pane"
import { Sidebar } from "@/shell/sidebar"
import { ShortcutsDialog } from "@/shortcuts/shortcuts-dialog"
import {
  shortcutPanelId,
  useDashboardShortcuts,
} from "@/shortcuts/use-dashboard-shortcuts"
import {
  findPanel,
  isDataPanel,
  panelTitle,
  sourcePanelOf,
  type Dashboard,
  type DataPanel,
  type LayoutPanel,
} from "@/spec"

import { ActivityToggle } from "./activity-toggle"
import {
  cursorOverride,
  cursorSyncLabel,
  nextCursorSync,
  savedCursorSync,
  useCursorSyncShortcut,
} from "./cursor-sync"
import { CanvasTools, useCanvasTool } from "./canvas-tools"
import { DashboardGrid, type PanelItem } from "./dashboard-grid"
import { DashboardLinks } from "./dashboard-links"
import { DashboardPageSkeleton, pageClass } from "./dashboard-page-skeleton"
import { DashboardTopRow } from "./dashboard-top-row"
import { DeleteDashboardDialog } from "./delete-dashboard-dialog"
import { duplicateBlockReason, duplicateTargets } from "./duplicate"
import { DuplicateDashboardDialog } from "./duplicate-dashboard-dialog"
import { detailsBlockReason } from "./edit/details"
import { DetailsDialog } from "./edit/details-dialog"
import {
  findMoreMenuButton,
  useGridEditable,
  useIsDirty,
  useLayoutStore,
} from "./edit/layout-store"
import { LayoutAnnouncer } from "./edit/panel-move-handle"
import { saveProblemFrom } from "./edit/save-problem"
import { UnsavedBar } from "./edit/unsaved-bar"
import { UnsavedChangesGuard } from "./edit/unsaved-changes-guard"
import {
  useDashboardEvents,
  type LiveStatus,
} from "./live/use-dashboard-events"
import { downloadSpec } from "./export-spec"
import { KioskHint } from "./kiosk/kiosk-hint"
import { useKiosk } from "./kiosk/use-kiosk"
import {
  requestFullscreen,
  useKioskFullscreen,
} from "./kiosk/use-kiosk-fullscreen"
import { MoreMenu } from "./more-menu"
import { noNewPanels, panelsAddedSince, type SeenVersion } from "./new-panels"
import { PanelContainer } from "./panel-container"
import { ProposalsButton } from "./proposals-button"
import { mixesDatasources } from "./panel-datasources"
import type { PanelSource } from "./panel-source"
import type { DashboardAccess } from "./permissions"
import {
  asDashboardSpec,
  dashboardVersionQuery,
  forgetDashboard,
  latestDashboardQuery,
  useAddVersion,
  type DashboardEnvelope,
} from "./queries"
import { RefreshPicker } from "./refresh-picker"
import { withResponsive } from "./responsive"
import { saveToast } from "./save-outcome"
import { interpolateTitle } from "./rows/interpolate-title"
import { useRowStore } from "./rows/row-store"
import { useSelectedPanel, useSelectionStore } from "./selection-store"
import {
  backFromSearch,
  panelEditorShowsNewerVersion,
  useDashboardSearch,
  useSetDashboardSearch,
  type DashboardSearch,
} from "./search"
import {
  effectiveRefresh,
  pickerQuickRanges,
  pickerRefreshOptions,
} from "./time-picker-settings"
import { TimeRangePicker } from "./time-range-picker"
import { unverifiedReasons } from "./unverified"
import { useVariables } from "./use-variables"
import { VariablesBar } from "./variables-bar"
import { VersionMenu } from "./version-menu"

function scrollPaneIntoView(id: string) {
  requestAnimationFrame(() =>
    document.getElementById(id)?.scrollIntoView({
      block: "nearest",
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "auto"
        : "smooth",
    })
  )
}

function expandRowOf(spec: Dashboard, dashboardId: string, panelId: string) {
  const row = findPanel(spec, panelId)?.row
  if (row === undefined) return
  const { collapsed, toggle } = useRowStore.getState()
  const initial = row.collapsed ?? false
  if (collapsed[dashboardId]?.[row.id] ?? initial) {
    toggle(dashboardId, row.id, initial)
  }
}

function layoutPanelOf(spec: Dashboard, id: string | undefined) {
  if (id === undefined) return undefined
  const panel = findPanel(spec, id)?.panel
  return panel === undefined || panel.type === "row" ? undefined : panel
}

function inspectablePanelOf(
  spec: Dashboard,
  id: string | undefined
): DataPanel | undefined {
  const panel = layoutPanelOf(spec, id)
  return panel !== undefined && isDataPanel(panel) && isRenderable(panel.type)
    ? panel
    : undefined
}

function withoutKiosk(state: DashboardSearch): DashboardSearch {
  return { ...state, kiosk: undefined, cycle: undefined, interval: undefined }
}

type Size = { width: number; height: number }

function ViewedPanel({
  onBack,
  children,
}: {
  onBack: () => void
  children: (size: Size) => ReactNode
}) {
  const box = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState<Size | null>(null)
  useLayoutEffect(() => {
    const element = box.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) =>
      setSize({
        width: Math.floor(entry.contentRect.width),
        height: Math.floor(entry.contentRect.height),
      })
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return (
    <div className="flex h-full min-h-[70svh] flex-col gap-2">
      <div>
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeftIcon aria-hidden="true" />
          Back to dashboard
        </Button>
      </div>
      <div ref={box} className="min-h-0 flex-1">
        {size ? children(size) : null}
      </div>
    </div>
  )
}

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
  const ensureLayout = useLayoutStore((state) => state.ensure)
  const gridBox = useRef<HTMLDivElement>(null)
  const gridEditable = useGridEditable(gridBox)
  const layoutDirty = useIsDirty()
  const sessionOpen = session !== null && session.dashboardId === dashboardId
  const editing = sessionOpen && search.version === undefined && layoutDirty
  const version = editing ? session.baseVersion : envelopeVersion
  const spec = editing ? session.baseSpec : envelopeSpec
  const unverified = useMemo(
    () =>
      unverifiedReasons(envelope.version.unverified, envelopeVersion, version),
    [envelope.version.unverified, envelopeVersion, version]
  )
  const panelHealth = usePanelHealth(
    dashboardId,
    version,
    editing ? "finish the layout first" : null
  )
  const actionContext = useMemo(
    () => ({
      dashboardId,
      version,
      latestVersion,
      canRun: permissions.edit,
    }),
    [dashboardId, version, latestVersion, permissions.edit]
  )
  const variables = useMemo(() => spec.variables ?? [], [spec])
  const timeOptions = useMemo(() => timeOptionsFor(spec), [spec])

  const source = useMemo<PanelSource>(
    () => ({ kind: "version", dashboardId, version }),
    [dashboardId, version]
  )
  const { states, selections, values, ready, blocking, select } = useVariables({
    source,
    variables,
    from: search.from,
    to: search.to,
    timezone: spec.timezone,
    urlVars: search.vars,
  })
  const mixedDatasources = useMemo(
    () => mixesDatasources(spec, selections),
    [spec, selections]
  )

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
  const refreshValue = effectiveRefresh(search.refresh, spec)
  const refreshMs = refreshToMs(refreshValue)

  const pinned = search.version !== undefined
  const [seen, setSeen] = useState<{
    latest: SeenVersion | undefined
    fresh: ReadonlySet<string>
  }>({ latest: undefined, fresh: noNewPanels })
  if (envelopeVersion !== seen.latest?.version) {
    const next = { version: envelopeVersion, spec: envelopeSpec }
    setSeen({
      latest: next,
      fresh:
        pinned || editing ? noNewPanels : panelsAddedSince(seen.latest, next),
    })
  }
  const { mutate: saveVersion } = useAddVersion(dashboardId)
  const toggleResponsive = useCallback(
    (panel: LayoutPanel) => {
      const fixed = panel.responsive === false
      const title = panelTitle(panel)
      saveVersion(
        {
          spec: withResponsive(spec, panel.id, fixed),
          summary: fixed
            ? `Make "${title}" responsive`
            : `Keep the size of "${title}"`,
          parent_version: version,
          propose: false,
        },
        {
          onSuccess: (outcome) => toast.success(saveToast(outcome)),
          onError: (failure) => {
            const problem = saveProblemFrom(
              failure,
              "Could not change the panel's size setting"
            )
            if (problem.kind === "conflict") {
              toast.error(
                "The dashboard changed since you opened it; reload and try again"
              )
            } else {
              toast.error(
                problem.kind === "invalid" ? problem.heading : problem.message
              )
            }
          },
        }
      )
    },
    [saveVersion, spec, version]
  )
  const open = useActivityStore((state) => state.open)
  const setOpen = useActivityStore((state) => state.setOpen)
  const kiosk = search.kiosk === true
  const viewed = layoutPanelOf(spec, search.viewPanel)
  const inspectPanel = inspectablePanelOf(spec, search.inspect)
  const inspecting = inspectPanel !== undefined && !editing
  const panelInUrl = search.edit !== undefined && search.panel !== undefined
  const mayOpenEditor = permissions.edit || permissions.comment
  const editingPanel = mayOpenEditor && panelInUrl
  const arranging =
    permissions.edit &&
    search.version === undefined &&
    gridEditable &&
    viewed === undefined &&
    !panelInUrl &&
    !kiosk
  useLayoutEffect(() => {
    if (arranging) ensureLayout(dashboardId, envelopeVersion, envelopeSpec)
  }, [
    arranging,
    sessionOpen,
    ensureLayout,
    dashboardId,
    envelopeVersion,
    envelopeSpec,
  ])
  const selection = useSelectedPanel(dashboardId)
  const selectedId =
    !kiosk &&
    viewed === undefined &&
    selection !== null &&
    layoutPanelOf(spec, selection) !== undefined
      ? selection
      : null
  const layout =
    sessionOpen && ((arranging && selectedId !== null) || editing)
      ? session.layout
      : undefined
  useEffect(() => {
    useSelectionStore.getState().clear()
  }, [dashboardId, search.version])
  const commentTool = useCanvasTool((state) => state.tool === "comment")
  const chatRequestId =
    panelInUrl || search.share === true || editing || inspecting || kiosk
      ? undefined
      : search.chat
  const threadInUrl = search.thread !== undefined
  const cardVisible =
    (open || panelInUrl || search.share === true || threadInUrl) &&
    !editing &&
    !inspecting &&
    !kiosk &&
    chatRequestId === undefined
  const cardLayout = useActivityLayout(editingPanel)
  const chatLayout = useChatPaneLayout()
  const inspectLayout = useActivityLayout(false)
  const [inspectTarget, setInspectTarget] = useState<InspectTarget | null>(null)
  const [shortcutsOpen, setShortcutsOpen] = useState(false)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const editGuard = useRef<EditGuard>(null)
  const [cursor] = useState(createSharedCursor)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [duplicating, setDuplicating] = useState(false)
  const folders = useQuery(foldersQuery()).data ?? []
  const duplicate = duplicateTargets(folders, envelope.dashboard.folder.id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
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

  const [linkedPanel, setLinkedPanel] = useState(search.panel)
  const clearLinkedPanel = useCallback(() => setLinkedPanel(undefined), [])
  const [focusesLinkedPanel] = useState(search.edit === undefined)
  useEffect(() => {
    if (linkedPanel !== undefined) expandRowOf(spec, dashboardId, linkedPanel)
  }, [linkedPanel, spec, dashboardId])

  const inspectedId = inspectPanel?.id
  const inspectStacked = inspectLayout.stacked
  useEffect(() => {
    if (inspectedId === undefined) return
    expandRowOf(spec, dashboardId, inspectedId)
    if (inspectStacked) {
      scrollPaneIntoView(inspectPaneId)
      return
    }
    requestAnimationFrame(() =>
      document
        .querySelector(`[data-panel-id="${CSS.escape(inspectedId)}"]`)
        ?.scrollIntoView({ block: "nearest" })
    )
  }, [inspectedId, inspectStacked, spec, dashboardId])

  const dropView = search.viewPanel !== undefined && viewed === undefined
  const dropInspect = search.inspect !== undefined && inspectPanel === undefined
  useEffect(() => {
    if (!dropView && !dropInspect) return
    setSearch(
      (state) => ({
        ...state,
        viewPanel: dropView ? undefined : state.viewPanel,
        inspect: dropInspect ? undefined : state.inspect,
        inspectTab: dropInspect ? undefined : state.inspectTab,
      }),
      { replace: true }
    )
  }, [dropView, dropInspect, setSearch])

  const layoutEditingWithPanel =
    editing &&
    (search.panel !== undefined ||
      search.edit !== undefined ||
      search.share === true ||
      search.viewPanel !== undefined ||
      search.inspect !== undefined)
  useEffect(() => {
    if (!layoutEditingWithPanel) return
    setSearch(
      (state) => ({
        ...state,
        panel: undefined,
        edit: undefined,
        share: false,
        viewPanel: undefined,
        inspect: undefined,
        inspectTab: undefined,
      }),
      { replace: true }
    )
  }, [layoutEditingWithPanel, setSearch])

  const closeInspect = useCallback(
    () =>
      setSearch((state) => ({
        ...state,
        inspect: undefined,
        inspectTab: undefined,
      })),
    [setSearch]
  )
  const closeView = useCallback(
    () => setSearch((state) => ({ ...state, viewPanel: undefined })),
    [setSearch]
  )

  useKiosk({ dashboardId, folderId: access.folder.id, search, grid: gridBox })
  const { offerFullscreen, leave: leaveKiosk } = useKioskFullscreen(kiosk, () =>
    setSearch(withoutKiosk)
  )
  const enterKiosk = (update: (state: DashboardSearch) => DashboardSearch) => {
    requestFullscreen()
    setSearch(update)
  }

  const timeHidden = spec.timePicker?.hidden === true
  const moveRange = (
    move: (now: number) => { from: string; to: string } | null
  ): (() => void) | undefined =>
    timeHidden
      ? undefined
      : () => {
          const next = move(Date.now())
          if (next) setSearch((state) => ({ ...state, ...next }))
        }
  const shortcutPanel = () =>
    layoutPanelOf(spec, shortcutPanelId() ?? undefined)
  useDashboardShortcuts({
    view: () => {
      if (viewed !== undefined) return closeView()
      const panel = shortcutPanel()
      if (panel && !editing) {
        setSearch((state) => ({ ...state, viewPanel: panel.id }))
      }
    },
    edit:
      mayOpenEditor && !editing
        ? () => {
            const panel = shortcutPanel()
            if (!panel || !isEditablePanel(panel)) return
            setOpen(true)
            setSearch((state) => ({ ...state, panel: panel.id, edit: "query" }))
          }
        : undefined,
    inspect: () => {
      const panel = inspectablePanelOf(spec, shortcutPanelId() ?? undefined)
      if (!panel || editing) return
      if (panel.id === search.inspect) return closeInspect()
      setSearch((state) => ({
        ...state,
        edit: undefined,
        share: false,
        inspect: panel.id,
        inspectTab: undefined,
      }))
    },
    close: inspecting
      ? closeInspect
      : viewed !== undefined
        ? closeView
        : kiosk
          ? leaveKiosk
          : commentTool
            ? () => useCanvasTool.getState().setTool("select")
            : selectedId !== null
              ? () => useSelectionStore.getState().clear()
              : undefined,
    zoomOut: moveRange((now) =>
      zoomOut(effectiveFrom, effectiveTo, now, timeOptions)
    ),
    shiftBack: moveRange((now) =>
      shift(effectiveFrom, effectiveTo, now, -1, timeOptions)
    ),
    shiftForward: moveRange((now) =>
      shift(effectiveFrom, effectiveTo, now, 1, timeOptions)
    ),
    refresh: () =>
      void queryClient.invalidateQueries({
        queryKey: dashboardKeys.panels(dashboardId, version),
      }),
    tvMode: kiosk
      ? undefined
      : () => enterKiosk((state) => ({ ...state, kiosk: true })),
    fullscreen: offerFullscreen ? requestFullscreen : undefined,
    help: () => setShortcutsOpen(true),
  })

  const closeCard = useCallback(() => {
    setOpen(false)
    if (
      search.panel === undefined &&
      search.edit === undefined &&
      search.share !== true &&
      search.chat === undefined &&
      search.thread === undefined
    ) {
      return
    }
    setSearch((state) => ({
      ...state,
      panel: undefined,
      edit: undefined,
      share: false,
      chat: undefined,
      thread: undefined,
    }))
  }, [
    setOpen,
    setSearch,
    search.panel,
    search.edit,
    search.share,
    search.chat,
    search.thread,
  ])

  const toggleCard = useCallback(() => {
    if (cardVisible) {
      closeCard()
      return
    }
    setOpen(true)
    if (search.chat !== undefined) {
      setSearch((state) => ({ ...state, chat: undefined }))
    }
    if (cardLayout.stacked) scrollPaneIntoView(activityCardId)
  }, [
    cardVisible,
    closeCard,
    setOpen,
    setSearch,
    search.chat,
    cardLayout.stacked,
  ])

  const toggleShare = useCallback(() => {
    const sharing = search.share === true
    const apply = () => {
      setSearch((state) => ({ ...state, edit: undefined, share: !sharing }))
      if (!sharing && cardLayout.stacked) scrollPaneIntoView(activityCardId)
    }
    if (editGuard.current) editGuard.current.leaveThen(apply)
    else apply()
  }, [search.share, setSearch, cardLayout.stacked])

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

  const controls = [
    <div key="time" className="flex items-center gap-1.5">
      <TimeRangePicker
        dashboardId={dashboardId}
        version={version}
        from={effectiveFrom}
        to={effectiveTo}
        quickRanges={pickerQuickRanges(spec)}
        hidden={spec.timePicker?.hidden === true}
      />
      {spec.timePicker?.hidden ? null : (
        <RefreshPicker
          value={refreshValue}
          choices={pickerRefreshOptions(spec)}
        />
      )}
    </div>,
  ]
  const actions: ReactNode[] = [
    <DashboardHealthButton
      key="health"
      dashboardId={dashboardId}
      blockedReason={editing ? "finish the layout first" : null}
    />,
    <ProposalsButton key="proposals" dashboardId={dashboardId} />,
    <MoreMenu
      key="more"
      mayEdit={permissions.edit}
      detailsBlockedBy={detailsBlockReason({
        pinned,
        editingLayout: editing,
        editingPanel,
      })}
      duplicateBlockedBy={duplicateBlockReason({
        pinned,
        editingLayout: editing,
      })}
      deleteBlockedBy={editing ? "finish the layout first" : null}
      onEditDetails={() => setDetailsOpen(true)}
      onDuplicate={
        duplicate.choices.length > 0 ? () => setDuplicating(true) : null
      }
      onDelete={() => setDeleting(true)}
      onExport={() => downloadSpec(spec, spec.title, version)}
      onTvMode={() => enterKiosk((state) => ({ ...state, kiosk: true }))}
      onTvModeFolder={
        access.folder.personal
          ? null
          : () =>
              enterKiosk((state) => ({
                ...state,
                kiosk: true,
                cycle: "folder",
              }))
      }
      cursorSync={cursorSync}
      onCursorSyncChange={chooseCursorSync}
    />,
    <StarToggle key="star" dashboardId={dashboardId} />,
    <ShareToggle
      key="share"
      pressed={search.share === true && !editing}
      disabled={editing}
      onToggle={toggleShare}
    />,
    <ActivityToggle
      key="activity"
      pressed={cardVisible}
      openCount={openTotal}
      disabled={editing}
      onToggle={toggleCard}
    />,
  ]

  const renderPanel = (item: PanelItem, size: Size, viewing = false) => (
    <PanelContainer
      dashboardId={dashboardId}
      source={source}
      cursor={cursor}
      panel={item.panel}
      reused={
        isDataPanel(item.panel) ? sourcePanelOf(spec, item.panel) : undefined
      }
      variables={variables}
      title={item.title}
      repeatOf={item.key === item.panel.id ? undefined : item.panel.id}
      width={size.width}
      height={size.height}
      from={search.from}
      to={search.to}
      timezone={spec.timezone}
      rangeFrom={effectiveFrom}
      rangeTo={effectiveTo}
      selections={item.selections}
      selectionsReady={ready}
      blockedBy={blockedBy}
      onRetryBlocked={onRetryBlocked}
      refreshMs={refreshMs}
      mixedDatasources={mixedDatasources}
      editable={
        arranging &&
        layout !== undefined &&
        item.first &&
        item.panel.id === selectedId
      }
      layoutSession={editing}
      openComments={item.first ? (openByPanel.get(item.panel.id) ?? 0) : 0}
      unverifiedReason={unverified.panels.get(item.panel.id)}
      health={
        item.first && !unverified.panels.has(item.panel.id)
          ? panelHealth.get(item.panel.id)
          : undefined
      }
      highlighted={item.panel.id === search.panel || item.panel.id === flashed}
      selected={item.first && !viewing && item.panel.id === selectedId}
      editingPanel={
        editingPanel && item.first && item.panel.id === search.panel
      }
      canComment={permissions.comment}
      canEdit={permissions.edit}
      onToggleResponsive={
        permissions.edit && !editing && !pinned ? toggleResponsive : undefined
      }
      focusOnRender={
        focusesLinkedPanel && item.first && item.panel.id === linkedPanel
      }
      onFocused={clearLinkedPanel}
      composerRef={composerRef}
      viewing={viewing}
      inspected={inspecting && item.first && item.panel.id === inspectedId}
      onInspectTarget={setInspectTarget}
    />
  )

  return (
    <div className="flex min-h-svh bg-chrome">
      {kiosk ? null : <Sidebar rail />}
      <div
        className={cn(
          pageClass(
            (cardVisible && cardLayout.stacked) ||
              (chatRequestId !== undefined && chatLayout.stacked) ||
              (inspecting && inspectLayout.stacked)
          ),
          "min-w-0 flex-1"
        )}
      >
        <div className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
          {kiosk ? null : (
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
              actions={actions}
            />
          )}
          {kiosk ? null : (
            <>
              <VariablesBar
                states={states}
                unverified={unverified.variables}
                onSelect={select}
              />
              <DashboardLinks
                links={spec.links}
                dashboardId={dashboardId}
                timeOptions={timeOptions}
                scope={{
                  vars: selections,
                  from: effectiveFrom,
                  to: effectiveTo,
                }}
              />
            </>
          )}
          <LayoutAnnouncer />
          <main
            ref={gridBox}
            className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4"
          >
            <ActionProvider value={actionContext}>
              {viewed ? (
                <ViewedPanel onBack={closeView}>
                  {(size) =>
                    renderPanel(
                      {
                        kind: "panel",
                        key: viewed.id,
                        panel: viewed,
                        title: interpolateTitle(panelTitle(viewed), selections),
                        selections,
                        first: true,
                        gridPos: viewed.gridPos,
                      },
                      size,
                      true
                    )
                  }
                </ViewedPanel>
              ) : (
                <DashboardGrid
                  dashboardId={dashboardId}
                  spec={spec}
                  selections={selections}
                  values={values}
                  editable={arranging && layout !== undefined}
                  layout={layout}
                  onLayoutChange={layout ? setLayout : undefined}
                  selectedId={selectedId}
                  newPanelIds={editing ? undefined : seen.fresh}
                  renderPanel={(item, size) => renderPanel(item, size)}
                />
              )}
            </ActionProvider>
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
            canEdit={mayOpenEditor}
            folder={access.folder}
            canManage={permissions.manage}
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
        {chatRequestId !== undefined ? (
          <ChatPane requestId={chatRequestId} onClose={closeCard} />
        ) : null}
        {inspecting ? (
          <InspectPane
            title={panelTitle(inspectPanel)}
            tab={search.inspectTab ?? "data"}
            target={
              inspectTarget?.panel.id === inspectPanel.id ? inspectTarget : null
            }
            variables={variables}
            zone={displayZone(inspectPanel.timezone, spec.timezone)}
            onClose={closeInspect}
          />
        ) : null}
        <UnsavedBar dashboardId={dashboardId} />
        {kiosk ? null : (
          <CanvasTools
            permissions={permissions}
            idle={!editing}
            composerRef={composerRef}
          />
        )}
        {kiosk ? <KioskHint offerFullscreen={offerFullscreen} /> : null}
        <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
        <DetailsDialog
          dashboardId={dashboardId}
          spec={envelopeSpec}
          version={envelopeVersion}
          open={detailsOpen}
          onOpenChange={setDetailsOpen}
          finalFocus={() => findMoreMenuButton() ?? true}
        />
        <DuplicateDashboardDialog
          source={{
            id: dashboardId,
            title: spec.title,
            version: latestVersion,
          }}
          targets={duplicate}
          open={duplicating}
          onOpenChange={setDuplicating}
          finalFocus={() => findMoreMenuButton() ?? true}
        />
        <DeleteDashboardDialog
          dashboard={{
            id: dashboardId,
            title: spec.title,
            versions: latestVersion,
          }}
          open={deleting}
          onOpenChange={setDeleting}
          finalFocus={() => findMoreMenuButton() ?? true}
          onDeleted={async (alreadyGone) => {
            toast.success(
              alreadyGone
                ? `${spec.title} was already deleted`
                : `Deleted ${spec.title}`
            )
            await navigate({ to: "/dashboards", ignoreBlocker: true })
            forgetDashboard(queryClient, dashboardId)
          }}
        />
      </div>
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
  const { status: liveStatus } = useDashboardEvents(dashboardId, {
    pinnedVersion: search.version,
    toastVersions: !panelEditorShowsNewerVersion(search),
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
