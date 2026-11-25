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
import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { ArrowLeftIcon } from "lucide-react"
import { toast } from "sonner"

import { ActivityCard } from "@/activity/activity-card"
import { announcePaneOpening } from "@/components/dock/dock-focus"
import type { EditGuard } from "@/activity/edit-pane"
import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { AnnotationActionsProvider } from "@/annotations/actions"
import { AnnotationControls } from "@/annotations/annotation-controls"
import { dashboardKeys } from "@/api/keys"
import { useComments } from "@/comments/queries"
import { openThreadCount, openThreadsByPanel } from "@/comments/threads"
import { Button } from "@/components/ui/button"
import { foldersQuery } from "@/folders/queries"
import { panelTitles } from "@/history/diff-groups"
import { DashboardHealthButton } from "@/health/dashboard-health-button"
import { usePanelHealth } from "@/health/use-dashboard-health"
import {
  InspectPane,
  inspectPaneId,
  type InspectTarget,
} from "@/inspect/inspect-pane"
import { preferredZone, usePreferences } from "@/lib/preferences"
import {
  displayZone,
  isRangeTooLong,
  maxRangeDays,
  refreshToMs,
  timeOptionsFor,
} from "@/lib/time"
import { shift, zoomOut } from "@/lib/time-shift"
import { madeByBreadcrumb } from "@/lib/origin"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { isEditablePanel } from "@/panel-editor/draft"
import { isRenderable } from "@/panels"
import { ActionProvider } from "@/panels/actions/action-context"
import { createSharedCursor, type CursorSync } from "@/panels/shared-cursor"
import { PinToggle } from "@/pins/pin-toggle"
import { ShareToggle } from "@/share/share-pane"
import { InboxPanel } from "@/inbox/inbox-panel"
import { closeInboxIfOpen } from "@/inbox/inbox-panel-store"
import { CommandPalette } from "@/shell/command-palette"
import { PhoneBar } from "@/shell/phone-bar"
import { Sidebar } from "@/shell/sidebar"
import { PanelHostContext } from "@/shell/side-panel"
import { ShortcutsHost } from "@/shortcuts/shortcuts-dialog"
import { useShortcutsStore } from "@/shortcuts/shortcuts-store"
import {
  shortcutPanelId,
  shortcutTarget,
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
  askAboutThisPanel,
  commentOnThisPanel,
  conversationActionsFor,
  focusComment,
} from "./panel-actions"
import { ViewerNames } from "./viewer-names"
import {
  cursorOverride,
  cursorSyncLabel,
  nextCursorSync,
  savedCursorSync,
  useCursorSyncShortcut,
} from "./cursor-sync"
import { CanvasTools, useCanvasTool } from "./canvas-tools"
import { useOpenChat } from "@/comments/ask-popover"
import { useCanvasGate } from "@/comments/canvas-composer"
import { CanvasHostProvider, type CanvasHost } from "@/comments/canvas-host"
import { askKeyAction } from "./ask-key"
import { commentKeyAction, takesCanvasComments } from "./comment-key"
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
import type { VariableSource } from "./panel-source"
import {
  contentChange,
  mayEditBesidesContent,
  type DashboardAccess,
} from "./permissions"
import {
  asDashboardSpec,
  dashboardVersionQuery,
  forgetDashboard,
  latestDashboardQuery,
  useAddVersion,
  type DashboardEnvelope,
} from "./queries"
import { useAddPanel } from "./add-panel-action"
import { SpecJsonPane } from "./spec-json/spec-json-pane"
import { usePanelLayoutActions } from "./panel-layout-actions"
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
import { ZonePicker } from "./zone-picker"
import { unverifiedReasons } from "./unverified"
import { useVariables } from "./use-variables"
import { VariablesBar } from "./variables-bar"
import { ExportView } from "./print/export-view"
import { useExportStore } from "./print/export-store"
import { VersionMenu } from "./version-menu"
import { useViewsText } from "./views"
import { activityCardId } from "./pane-openers"
import {
  arrangingPaneAfter,
  arrangingSetsPaneAside,
  dockOf,
  heldPaneOf,
  isSidePanelCard,
  type ArrangingPane,
} from "./dock"

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
  const { permissions, provisioned } = access
  const { principal } = useRouteContext({ from: "/_authed" })
  const content = contentChange(permissions, provisioned)
  const besidesContent = mayEditBesidesContent(permissions, provisioned)
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
  const preferences = usePreferences()
  const ownZone = spec.timezone ?? preferredZone(preferences.timeZone)
  const viewZone = search.tz ?? ownZone
  const timeOptions = useMemo(
    () => timeOptionsFor({ ...spec, timezone: viewZone }),
    [spec, viewZone]
  )

  const source = useMemo<VariableSource>(
    () => ({ kind: "version", dashboardId, version }),
    [dashboardId, version]
  )
  const { states, selections, values, ready, blocking, select } = useVariables({
    source,
    variables,
    from: search.from,
    to: search.to,
    timezone: viewZone,
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
  const rangeTooLong = useMemo(
    () => isRangeTooLong(effectiveFrom, effectiveTo, undefined, timeOptions),
    [effectiveFrom, effectiveTo, timeOptions]
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
  const layoutActions = usePanelLayoutActions(dashboardId, spec, version)
  const addPanel = useAddPanel(dashboardId, spec)
  const open = useActivityStore((state) => state.open)
  const setOpen = useActivityStore((state) => state.setOpen)
  const kiosk = search.kiosk === true
  const viewed = layoutPanelOf(spec, search.viewPanel)
  const inspectPanel = inspectablePanelOf(spec, search.inspect)
  const panelInUrl = search.edit !== undefined && search.panel !== undefined
  const mayOpenEditor = content !== "none"
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
  const storedSelection = useSelectedPanel(dashboardId)
  const selection =
    storedSelection !== null &&
    layoutPanelOf(spec, storedSelection) !== undefined
      ? storedSelection
      : null
  const selectedId = !kiosk && viewed === undefined ? selection : null
  const layout =
    sessionOpen && ((arranging && selectedId !== null) || editing)
      ? session.layout
      : undefined
  useEffect(() => {
    useSelectionStore.getState().clear()
  }, [dashboardId, search.version])
  const selectionGone = storedSelection !== null && selection === null
  useEffect(() => {
    if (selectionGone) useSelectionStore.getState().clear()
  }, [selectionGone])
  const tool = useCanvasTool((state) => state.tool)
  const canvasGate = useCanvasGate(permissions.comment, editing)
  const pressArranges = arranging && (tool === "select" || editing)
  const toolsShown = permissions.comment
  const [specOpen, setSpecOpen] = useState(false)
  const dockState = {
    open,
    layoutEditing: editing,
    inspectable: inspectPanel !== undefined,
    specOpen,
  }
  const dock = dockOf(search, dockState)
  const heldPane = heldPaneOf(search, dockState)
  const cardLayout = useActivityLayout(editingPanel)
  const paneOpenings = useActivityStore((state) => state.openings)
  const [seenOpenings, setSeenOpenings] = useState(paneOpenings)
  const paneAddress = [search.tab, search.chat, search.thread].join("|")
  const [seenPaneAddress, setSeenPaneAddress] = useState(paneAddress)
  const [arrangingPane, setArrangingPane] = useState<ArrangingPane>(null)
  const nextArrangingPane = arrangingPaneAfter(arrangingPane, {
    arranging: selection !== null || editing,
    setsAside:
      selectedId !== null &&
      pressArranges &&
      arrangingSetsPaneAside(dock, !cardLayout.stacked),
    reopened: seenOpenings !== paneOpenings || seenPaneAddress !== paneAddress,
  })
  if (seenOpenings !== paneOpenings) setSeenOpenings(paneOpenings)
  if (seenPaneAddress !== paneAddress) setSeenPaneAddress(paneAddress)
  if (nextArrangingPane !== arrangingPane) {
    setArrangingPane(nextArrangingPane)
  }
  const paneAside = nextArrangingPane === "aside"
  const inspecting = dock === "inspect"
  const specShown = dock === "spec"
  const cardView = isSidePanelCard(dock) && !paneAside ? dock : null
  const mountedCard = isSidePanelCard(dock) ? dock : heldPane
  const cardVisible = cardView !== null
  const { chatId: askChatId, select: selectAskChat } = useOpenChat(
    dashboardId,
    cardView
  )
  const canvasHost = useMemo<CanvasHost>(
    () => ({
      subject: { kind: "dashboard", id: dashboardId },
      gate: canvasGate,
      chat: { chatId: askChatId, select: selectAskChat },
    }),
    [dashboardId, canvasGate, askChatId, selectAskChat]
  )
  const inspectLayout = useActivityLayout(false)
  const [inspectTarget, setInspectTarget] = useState<InspectTarget | null>(null)
  const [panelHost, setPanelHost] = useState<HTMLDivElement | null>(null)
  const setShortcutsOpen = useShortcutsStore((state) => state.setOpen)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const editGuard = useRef<EditGuard>(null)
  const [cursor] = useState(createSharedCursor)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [duplicating, setDuplicating] = useState(false)
  const folders = useQuery(foldersQuery()).data ?? []
  const viewsText = useViewsText(dashboardId)
  const exporting = useExportStore((state) => state.mode !== null)
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

  const closeSpec = useCallback(() => setSpecOpen(false), [])

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
  const agentAvailable = useAgentInfo().data?.available === true
  const shortcutPanel = () =>
    layoutPanelOf(spec, shortcutPanelId() ?? undefined)
  const shortcutPlace = () => {
    const target = shortcutTarget()
    const panel = layoutPanelOf(spec, target?.id ?? undefined)
    const onCanvas = takesCanvasComments({
      canComment: permissions.comment,
      onTv: kiosk,
      viewing: viewed !== undefined,
      editingPanel: editingPanel && panel?.id === search.panel,
      copy: target?.copy === true,
    })
    return { panel, onCanvas }
  }
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
    comment: () => {
      const { panel, onCanvas } = shortcutPlace()
      const action = commentKeyAction({
        commenting: tool === "comment",
        panelId: panel?.id,
        canComment: permissions.comment,
        editing,
        onCanvas,
      })
      switch (action.kind) {
        case "leaveMode":
          return useCanvasTool.getState().leave("select")
        case "enterMode":
          return useCanvasTool.getState().leave("comment")
        case "panel":
          commentOnThisPanel(setSearch, action.panelId, onCanvas, composerRef)
          return
        case "pane":
          return focusComment(setSearch, composerRef)
      }
    },
    ask: () => {
      const { panel, onCanvas } = shortcutPlace()
      const action = askKeyAction({
        asking: tool === "ask",
        panelId: panel?.id,
        panelAskable:
          panel !== undefined &&
          conversationActionsFor(panel, permissions.comment, agentAvailable)
            .ask,
        mayAsk: permissions.comment && agentAvailable,
        editing,
        onCanvas,
      })
      switch (action.kind) {
        case "leaveMode":
          return useCanvasTool.getState().leave("select")
        case "enterMode":
          return useCanvasTool.getState().leave("ask")
        case "panel":
          askAboutThisPanel(setSearch, action.panelId, onCanvas)
          return
        case "none":
          return
      }
    },
    inspect: () => {
      const panel = inspectablePanelOf(spec, shortcutPanelId() ?? undefined)
      if (!panel || editing) return
      if (closeInboxIfOpen() && panel.id === search.inspect) return
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
          : tool !== "select"
            ? () => useCanvasTool.getState().leave("select")
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
      search.tab === undefined &&
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
      tab: undefined,
      chat: undefined,
      thread: undefined,
    }))
  }, [
    setOpen,
    setSearch,
    search.panel,
    search.edit,
    search.share,
    search.tab,
    search.chat,
    search.thread,
  ])

  const toggleCard = useCallback(() => {
    if (cardVisible) {
      closeCard()
      return
    }
    announcePaneOpening()
    setOpen(true)
    if (cardLayout.stacked) scrollPaneIntoView(activityCardId)
  }, [cardVisible, closeCard, setOpen, cardLayout.stacked])

  const toggleShare = useCallback(() => {
    const sharing = search.share === true
    const apply = () => {
      if (!sharing) announcePaneOpening()
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

  const annotationPanelTitle = (id: string) =>
    panelTitles(envelopeSpec).get(id) ?? id
  const controls = [
    <AnnotationControls key="annotations" />,
    <div key="time" className="flex flex-wrap items-center gap-1.5">
      <TimeRangePicker
        dashboardId={dashboardId}
        version={version}
        from={effectiveFrom}
        to={effectiveTo}
        quickRanges={pickerQuickRanges(spec)}
        hidden={spec.timePicker?.hidden === true}
        timezone={ownZone}
        defaultRange={
          permissions.edit &&
          detailsBlockReason({
            pinned,
            editingLayout: editing,
            editingPanel,
          }) === null
            ? { spec: envelopeSpec, version: envelopeVersion }
            : null
        }
      />
      {spec.timePicker?.hidden ? null : (
        <ZonePicker value={viewZone ?? "browser"} dashboardZone={ownZone} />
      )}
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
      dashboardId={dashboardId}
      mayEdit={permissions.edit}
      mayDelete={besidesContent}
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
      exportBlockedBy={editing ? "finish the layout first" : null}
      onEditDetails={() => setDetailsOpen(true)}
      onAddPanel={addPanel}
      onEditJson={() => {
        announcePaneOpening()
        setSpecOpen(true)
      }}
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
    <PinToggle key="pin" dashboardId={dashboardId} />,
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
      timezone={viewZone}
      rangeFrom={effectiveFrom}
      rangeTo={effectiveTo}
      selections={item.selections}
      selectionsReady={ready}
      blockedBy={blockedBy}
      onRetryBlocked={onRetryBlocked}
      refreshMs={refreshMs}
      mixedDatasources={mixedDatasources}
      editable={
        pressArranges &&
        layout !== undefined &&
        item.first &&
        item.panel.id === selectedId
      }
      arrangeable={arranging && item.first}
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
      onCanvas={takesCanvasComments({
        canComment: permissions.comment,
        onTv: kiosk,
        viewing,
        editingPanel: editingPanel && item.panel.id === search.panel,
        copy: item.key !== item.panel.id,
      })}
      canPropose={content === "propose"}
      onToggleResponsive={
        permissions.edit && !editing && !pinned ? toggleResponsive : undefined
      }
      layoutActions={
        permissions.edit && !editing && !pinned ? layoutActions : undefined
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

  const annotationActions = {
    dashboardId,
    zone: displayZone(undefined, viewZone),
    panelTitle: annotationPanelTitle,
    canEdit: besidesContent && !kiosk,
    principal,
  }

  if (exporting) {
    return (
      <AnnotationActionsProvider {...annotationActions}>
        <ExportView
          spec={spec}
          folder={access.folder.title}
          version={version}
          from={effectiveFrom}
          to={effectiveTo}
          states={states}
          selections={selections}
          values={values}
          actions={actionContext}
          renderPanel={(item, size) => renderPanel(item, size)}
        />
      </AnnotationActionsProvider>
    )
  }

  return (
    <PanelHostContext value={panelHost}>
      <CanvasHostProvider value={canvasHost}>
        <AnnotationActionsProvider {...annotationActions}>
          <div className="flex min-h-svh bg-chrome">
            {kiosk ? null : <Sidebar rail />}
            <div className="flex min-w-0 flex-1 flex-col">
              {kiosk ? null : (
                <>
                  <PhoneBar title={spec.title} />
                  <CommandPalette />
                </>
              )}
              <div
                className={cn(
                  pageClass(
                    (cardVisible && cardLayout.stacked) ||
                      (inspecting && inspectLayout.stacked) ||
                      (specShown && inspectLayout.stacked)
                  ),
                  "@container min-w-0 @max-[1000px]:has-[[data-panel-host]>aside]:h-auto @max-[1000px]:has-[[data-panel-host]>aside]:min-h-svh @max-[1000px]:has-[[data-panel-host]>aside]:flex-wrap narrow:flex-1"
                )}
              >
                <div className="@container/dash flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
                  {kiosk ? null : (
                    <DashboardTopRow
                      title={spec.title}
                      folder={access.folder}
                      madeBy={madeByBreadcrumb(envelope.dashboard.creator)}
                      views={viewsText}
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
                      presence={<ViewerNames dashboardId={dashboardId} />}
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
                  {rangeTooLong && !kiosk ? (
                    <p
                      role="alert"
                      className="pr-2.5 pb-2.5 pl-4 text-[13px] text-destructive"
                    >
                      The time range is longer than {maxRangeDays} days, so
                      panels cannot load. Choose a shorter range.
                    </p>
                  ) : null}
                  <LayoutAnnouncer />
                  <main
                    ref={gridBox}
                    data-export-pane=""
                    className={cn(
                      "min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4",
                      toolsShown &&
                        !kiosk &&
                        !editingPanel &&
                        "pb-16 narrow:pb-2.5"
                    )}
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
                                title: interpolateTitle(
                                  panelTitle(viewed),
                                  selections
                                ),
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
                          editable={pressArranges && layout !== undefined}
                          layout={layout}
                          onLayoutChange={layout ? setLayout : undefined}
                          selectedId={selectedId}
                          newPanelIds={editing ? undefined : seen.fresh}
                          renderPanel={(item, size) => renderPanel(item, size)}
                        />
                      )}
                    </ActionProvider>
                  </main>
                  {kiosk || editingPanel ? null : (
                    <CanvasTools permissions={permissions} idle={!editing} />
                  )}
                </div>
                {mountedCard !== null ? (
                  <ActivityCard
                    view={mountedCard}
                    hidden={cardView === null}
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
                {inspecting && inspectPanel !== undefined ? (
                  <InspectPane
                    title={panelTitle(inspectPanel)}
                    tab={search.inspectTab ?? "data"}
                    target={
                      inspectTarget?.panel.id === inspectPanel.id
                        ? inspectTarget
                        : null
                    }
                    variables={variables}
                    zone={displayZone(inspectPanel.timezone, viewZone)}
                    onClose={closeInspect}
                  />
                ) : null}
                <div ref={setPanelHost} data-panel-host="" className="contents">
                  {kiosk ? null : <InboxPanel />}
                </div>
                {specOpen ? (
                  <SpecJsonPane
                    dashboardId={dashboardId}
                    spec={envelopeSpec}
                    version={envelopeVersion}
                    hidden={!specShown}
                    opener={findMoreMenuButton}
                    onClose={closeSpec}
                  />
                ) : null}
                {kiosk ? <ShortcutsHost /> : null}
                <UnsavedBar dashboardId={dashboardId} />
                {kiosk ? <KioskHint offerFullscreen={offerFullscreen} /> : null}
                <DetailsDialog
                  dashboardId={dashboardId}
                  spec={envelopeSpec}
                  version={envelopeVersion}
                  folder={envelope.dashboard.folder}
                  folders={folders}
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
          </div>
        </AnnotationActionsProvider>
      </CanvasHostProvider>
    </PanelHostContext>
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
