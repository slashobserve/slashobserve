import {
  memo,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type PointerEvent,
  type RefObject,
} from "react"
import { toast } from "sonner"

import { annotationsFor } from "@/annotations/markers"
import { useDashboardAnnotations, type Annotation } from "@/annotations/queries"
import type { Selections } from "@/api/keys"
import {
  CanvasComposer,
  CanvasLayer,
  useCanvasGate,
} from "@/comments/canvas-composer"
import { AskPins } from "@/comments/ask-pins"
import { AskPopover, useOpenChat } from "@/comments/ask-popover"
import { CanvasPins } from "@/comments/canvas-pins"
import { pinView } from "@/comments/canvas-target"
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { PanelHealthBadge } from "@/health/panel-health-badge"
import type { PanelHealth } from "@/health/use-dashboard-health"
import { downloadCsv } from "@/inspect/csv"
import type { InspectTarget } from "@/inspect/inspect-pane"
import { savePanelPng } from "@/inspect/png"
import { isRenderable, isStaticPanel, preloadRenderer } from "@/panels"
import {
  AnchorLayoutProvider,
  createAnchorLayoutStore,
} from "@/panels/anchor-layout"
import { createFieldResolver, panelLinks } from "@/panels/field-config"
import { linkScopeFor, PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import type { SharedCursor } from "@/panels/shared-cursor"
import { prepareFrames, type DatasourceTypeOf } from "@/panels/transforms"
import type { ReusedFilter } from "@/panels/transforms/ad-hoc-filter"
import { reusedFilterOf, reusePanel } from "@/panels/transforms/reuse-panel"
import { useGazetteers } from "@/panels/transforms/use-gazetteers"
import type { WireFrame } from "@/dashboards/queries"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { useHoveredPanel } from "@/shortcuts/hovered-panel"
import {
  isDataPanel,
  panelTitle,
  resolveDatasourceRef,
  type DataPanel,
  type LayoutPanel,
  type Variable,
} from "@/spec"

import {
  activeTool,
  useCanvasTool,
  type CanvasDraft,
  type OpenTurn,
} from "./canvas-tools"
import { conversationActionsFor } from "./panel-actions"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { panelDatasourceUids } from "./panel-datasources"
import type { PanelLayoutActions } from "./panel-layout-actions"
import { PanelMenu, type BlockMenuHost } from "./panel-menu"
import { PanelToolbar } from "./panel-toolbar"
import {
  panelDataKey,
  panelFingerprint,
  stateScope,
  type PanelSource,
} from "./panel-source"
import { seriesCapLabel } from "./series-cap"
import { useAnnotationTagFilter, useSetDashboardSearch } from "./search"
import { useSelectionStore } from "./selection-store"
import { useInView } from "./use-in-view"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { UnverifiedLabel } from "./unverified-label"
import { blockVariables } from "@/observations/panel-input"
import { sortSelections, VariableLoadError } from "./variables"

function useFramePreparer(
  panel: DataPanel | undefined,
  vars: Selections,
  reusedFilter: ReusedFilter | undefined
) {
  const datasources = useDatasources()
  const gazetteers = useGazetteers(panel?.transform)
  return useCallback(
    (frames: WireFrame[]) => {
      if (!panel) return []
      const typeOf: DatasourceTypeOf = (ref) => {
        const uid = resolveDatasourceRef(ref, vars)
        return uid === undefined
          ? undefined
          : datasourceTypeOf(datasources.data, uid)
      }
      return prepareFrames(
        panel,
        frames,
        typeOf,
        reusedFilter,
        vars,
        gazetteers
      )
    },
    [panel, vars, reusedFilter, datasources.data, gazetteers]
  )
}

function ownsDraft(
  draft: CanvasDraft | null,
  panelId: string,
  surfaceId: string,
  repeatOf: string | undefined
): draft is CanvasDraft {
  if (draft === null || draft.panelId !== panelId) return false
  return draft.surface === null
    ? repeatOf === undefined
    : draft.surface === surfaceId
}

function spotKeyOf(spot: Pick<CanvasDraft, "panelId" | "at">): string {
  return `${spot.panelId}:${spot.at?.x ?? ""}:${spot.at?.y ?? ""}`
}

function ownsTurn(
  turn: OpenTurn | null,
  panelId: string,
  surfaceId: string,
  repeatOf: string | undefined
): turn is OpenTurn {
  if (turn === null || turn.panelId !== panelId) return false
  return turn.surface === null
    ? repeatOf === undefined
    : turn.surface === surfaceId
}

function focusPanel(root: HTMLElement | null) {
  const article = root?.querySelector<HTMLElement>("article")
  if (!article) return
  article.tabIndex = -1
  article.scrollIntoView({ block: "center", behavior: "auto" })
  article.focus({ preventScroll: true })
}

const interactiveTarget =
  'button, a, input, select, textarea, [role="menuitem"], [role="toolbar"]'

type PanelContainerProps = {
  dashboardId?: string
  source: PanelSource
  cursor: SharedCursor
  panel: LayoutPanel
  reused: DataPanel | undefined
  variables: readonly Variable[]
  title: string
  repeatOf: string | undefined
  width: number
  height: number
  from: string | undefined
  to: string | undefined
  timezone: string | undefined
  rangeFrom: string
  rangeTo: string
  selections: Selections
  selectionsReady: boolean
  blockedBy: string | undefined
  onRetryBlocked: () => void
  refreshMs: number | null
  mixedDatasources: boolean
  editable: boolean
  arrangeable?: boolean
  layoutSession: boolean
  openComments: number
  unverifiedReason: string | undefined
  health?: PanelHealth
  highlighted: boolean
  selected?: boolean
  editingPanel: boolean
  canComment: boolean
  canEdit: boolean
  onCanvas?: boolean
  canPropose: boolean
  onToggleResponsive?: (panel: LayoutPanel) => void
  layoutActions?: PanelLayoutActions
  focusOnRender: boolean
  onFocused: () => void
  composerRef: RefObject<HTMLTextAreaElement | null>
  annotations?: readonly Annotation[]
  viewing?: boolean
  inspected?: boolean
  onInspectTarget?: (target: InspectTarget | null) => void
  blockMenu?: BlockMenuHost
  onPinRange?: (fromMs: number, toMs: number) => void
}

export const PanelContainer = memo(function PanelContainer({
  dashboardId,
  source,
  cursor,
  panel: sourcePanel,
  reused,
  variables,
  title,
  repeatOf,
  width,
  height,
  from,
  to,
  timezone,
  rangeFrom,
  rangeTo,
  selections,
  selectionsReady,
  blockedBy,
  onRetryBlocked,
  refreshMs,
  mixedDatasources,
  editable,
  arrangeable = false,
  layoutSession,
  openComments,
  unverifiedReason,
  health,
  highlighted,
  selected = false,
  editingPanel,
  canComment,
  canEdit,
  onCanvas = false,
  canPropose,
  onToggleResponsive,
  layoutActions,
  focusOnRender,
  onFocused,
  composerRef,
  annotations: givenAnnotations,
  viewing = false,
  inspected = false,
  onInspectTarget,
  blockMenu,
  onPinRange,
}: PanelContainerProps) {
  const panel = useMemo(
    () =>
      title === panelTitle(sourcePanel)
        ? sourcePanel
        : { ...sourcePanel, title },
    [sourcePanel, title]
  )
  const ref = useRef<HTMLDivElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)
  const surfaceId = useId()
  const [layoutStore] = useState(createAnchorLayoutStore)
  const { seen, inView } = useInView(ref)
  const [tableView, setTableView] = useState(false)
  const setSearch = useSetDashboardSearch()
  const canvasTool = useCanvasTool((state) => state.tool)
  const selectTool = canvasTool === "select"
  const agentAvailable = useAgentInfo().data?.available === true
  const { width: bodyWidth, height: bodyHeight } = panelBodySize(
    Math.floor(width),
    Math.floor(height)
  )
  const body = useMemo(
    () => ({ width: bodyWidth, height: bodyHeight }),
    [bodyWidth, bodyHeight]
  )
  const dataPanel = useMemo(
    () => (isDataPanel(panel) ? reusePanel(panel, reused) : undefined),
    [panel, reused]
  )
  const reusedFilter = useMemo(
    () =>
      isDataPanel(panel)
        ? reusedFilterOf(panel, reused, variables, selections)
        : undefined,
    [panel, reused, variables, selections]
  )
  const observationVariables = useMemo(
    () =>
      isDataPanel(panel) ? blockVariables(panel, variables, selections) : [],
    [panel, variables, selections]
  )
  const supported = isRenderable(panel.type)

  useEffect(() => {
    if (seen) preloadRenderer(panel.type)
  }, [seen, panel.type])

  useEffect(() => {
    if (!focusOnRender) return
    focusPanel(ref.current)
    onFocused()
  }, [focusOnRender, onFocused])

  const requested = dataPanel ?? panel
  const maxDataPoints = maxDataPointsFor(requested, body.width)
  const query = usePanelData({
    source,
    panel: requested,
    from,
    to,
    timezone,
    vars: selections,
    maxDataPoints,
    refreshMs,
    enabled: dataPanel !== undefined && supported && seen && selectionsReady,
    inView,
  })
  const inspectKey = useMemo(
    () =>
      inspected
        ? panelDataKey(
            source,
            requested.id,
            source.kind === "build" ? panelFingerprint(requested) : "",
            { from, to, selections: sortSelections(selections), maxDataPoints }
          )
        : undefined,
    [inspected, source, requested, from, to, selections, maxDataPoints]
  )

  const fetchedAnnotations = useDashboardAnnotations({
    source,
    from: rangeFrom,
    to: rangeTo,
    timezone,
    refreshMs,
  })
  const annotationTags = useAnnotationTagFilter()
  const allAnnotations = givenAnnotations ?? fetchedAnnotations
  const annotations = useMemo(
    () => annotationsFor(allAnnotations ?? [], panel.id, annotationTags),
    [allAnnotations, panel.id, annotationTags]
  )

  const fields = useMemo(
    () => (dataPanel ? createFieldResolver(dataPanel, selections) : undefined),
    [dataPanel, selections]
  )

  const prepare = useFramePreparer(dataPanel, selections, reusedFilter)
  const inspectFrames = useMemo(
    () => (inspected && query.data ? prepare(query.data.frames) : undefined),
    [inspected, query.data, prepare]
  )
  const panelName = panelTitle(panel)
  const refetch = query.refetch
  useEffect(() => {
    if (!dataPanel || !onInspectTarget || inspectKey === undefined) return
    onInspectTarget({
      panel: dataPanel,
      title: panelName,
      data: query.data,
      frames: inspectFrames,
      queryKey: inspectKey,
      selections,
      body: bodyRef,
      refetch: () => void refetch(),
    })
  }, [
    dataPanel,
    onInspectTarget,
    panelName,
    query.data,
    inspectKey,
    inspectFrames,
    selections,
    refetch,
  ])
  useEffect(() => {
    if (!inspected || !onInspectTarget) return
    return () => onInspectTarget(null)
  }, [inspected, onInspectTarget])

  const populated = useMemo(
    () =>
      dataPanel !== undefined &&
      fields !== undefined &&
      query.data !== undefined &&
      panelHasData(
        dataPanel,
        query.data.frames,
        fields,
        reusedFilter,
        selections
      ),
    [dataPanel, fields, query.data, reusedFilter, selections]
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

  const sourceKind = source.kind
  const onTimeRangeSelect = useCallback(
    (fromMs: number, toMs: number) => {
      if (sourceKind === "block") onPinRange?.(fromMs, toMs)
      if (sourceKind === "build" || sourceKind === "block") return
      setSearch((state) => ({
        ...state,
        from: String(Math.round(fromMs)),
        to: String(Math.round(toMs)),
      }))
    },
    [setSearch, sourceKind, onPinRange]
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

  const panelId = panel.id
  const onPointerEnter = useCallback(
    () => useHoveredPanel.getState().enter(panelId, repeatOf !== undefined),
    [panelId, repeatOf]
  )
  const onPointerLeave = useCallback(
    () => useHoveredPanel.getState().leave(panelId),
    [panelId]
  )
  useEffect(() => () => useHoveredPanel.getState().leave(panelId), [panelId])

  const permissions = useMemo(
    () => ({ comment: canComment, edit: canEdit, propose: canPropose }),
    [canComment, canEdit, canPropose]
  )
  const selectPanel = useMemo(
    () =>
      dashboardId === undefined
        ? undefined
        : () => useSelectionStore.getState().select(dashboardId, panelId),
    [dashboardId, panelId]
  )
  const pointing = onCanvas
    ? activeTool(canvasTool, {
        ...conversationActionsFor(panel, canComment, agentAvailable),
        idle: !layoutSession,
      })
    : "select"
  const draft = useCanvasTool((state) =>
    ownsDraft(state.draft, panelId, surfaceId, repeatOf) ? state.draft : null
  )
  const openTurn = useCanvasTool((state) =>
    ownsTurn(state.openTurn, panelId, surfaceId, repeatOf)
      ? state.openTurn
      : null
  )
  const openChat = useOpenChat()
  const askSpot = openTurn ?? (draft?.tool === "ask" ? draft : null)
  const gateInput = useCanvasGate(canComment, layoutSession)
  const shownWindow = useMemo(
    () => ({ from: rangeFrom, to: rangeTo }),
    [rangeFrom, rangeTo]
  )
  const onPointerDownCapture = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || viewing) return
    const target = event.target as Element
    const pressesTitle =
      pointing === "select" && target.closest("[data-panel-title]") !== null
    if (!pressesTitle && target.closest(interactiveTarget)) return
    if (pointing !== "select") return
    if (!target.closest("[data-panel-body]")) selectPanel?.()
  }

  const data = query.data
  const downloadsReady = state === "ready"
  const onDownloadCsv = useMemo(
    () =>
      dataPanel === undefined
        ? undefined
        : () => {
            if (data) downloadCsv(prepare(data.frames), panelName)
          },
    [dataPanel, data, prepare, panelName]
  )
  const onSavePng = useCallback(() => {
    const element = bodyRef.current
    if (!element) return
    savePanelPng(element, panelName).catch(() =>
      toast.error("Could not save the panel as PNG")
    )
  }, [panelName])

  const panelMenu = (inToolbar: boolean) => (
    <PanelMenu
      panel={panel}
      range={{ from: rangeFrom, to: rangeTo }}
      selections={selections}
      tableView={tableView}
      onToggleTableView={() => setTableView((current) => !current)}
      idle={!layoutSession}
      viewing={viewing}
      onCanvas={onCanvas}
      onDownloadCsv={onDownloadCsv}
      onSavePng={onSavePng}
      downloadsReady={downloadsReady}
      canComment={canComment}
      canEdit={canEdit}
      canPropose={canPropose}
      onToggleResponsive={
        onToggleResponsive && !layoutSession
          ? () => onToggleResponsive(panel)
          : undefined
      }
      layoutActions={layoutSession ? undefined : layoutActions}
      composerRef={composerRef}
      inToolbar={inToolbar}
      observationVariables={observationVariables}
      block={blockMenu}
    />
  )

  return (
    <div
      ref={ref}
      className={cn(
        "relative size-full",
        pointing !== "select" && "cursor-crosshair **:cursor-crosshair"
      )}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      onPointerDownCapture={onPointerDownCapture}
    >
      <PanelChrome
        panel={panel}
        state={state}
        error={query.data ? query.error : (blockedError ?? query.error)}
        refreshing={query.isFetching && query.data !== undefined}
        staleError={query.isError && query.data ? query.error : undefined}
        onRetry={() => (blockedError ? onRetryBlocked() : void query.refetch())}
        dragHandle={editable}
        arrangeable={arrangeable}
        grabbable={arrangeable && selectTool}
        highlighted={highlighted}
        selected={selected}
        onSelect={viewing ? undefined : selectPanel}
        editing={editingPanel}
        repeatOf={repeatOf}
        datasourceLabel={
          mixedDatasources
            ? panelDatasourceUids(dataPanel ?? panel, selections).join(", ")
            : undefined
        }
        seriesCap={seriesCapLabel(query.data?.meta)}
        links={links}
        badge={
          <>
            {unverifiedReason ? (
              <UnverifiedLabel
                title={panelTitle(panel)}
                reason={unverifiedReason}
              />
            ) : null}
            {health ? (
              <PanelHealthBadge title={panelTitle(panel)} {...health} />
            ) : null}
            <PanelCommentBadge
              panelId={panel.id}
              title={panelTitle(panel)}
              count={openComments}
              disabled={layoutSession}
            />
          </>
        }
        menu={panelMenu(false)}
      >
        <AnchorLayoutProvider value={layoutStore}>
          <div
            ref={surfaceRef}
            data-canvas-surface=""
            className="relative size-full"
          >
            <div ref={bodyRef} className="size-full">
              {dataPanel && query.data ? (
                <PanelBody
                  panel={dataPanel}
                  data={query.data}
                  width={body.width}
                  height={body.height}
                  stateScope={stateScope(source)}
                  cursor={cursor}
                  onTimeRangeSelect={onTimeRangeSelect}
                  vars={selections}
                  timezone={timezone}
                  from={rangeFrom}
                  to={rangeTo}
                  asTable={tableView}
                  reusedFilter={reusedFilter}
                  annotations={annotations}
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
            </div>
            {onCanvas && dashboardId !== undefined && repeatOf === undefined ? (
              <CanvasPins
                dashboardId={dashboardId}
                panelId={panelId}
                panelTitle={panelName}
                store={layoutStore}
                body={body}
                view={pinView(state, tableView)}
                gateInput={gateInput}
              />
            ) : null}
            {onCanvas && dashboardId !== undefined && repeatOf === undefined ? (
              <AskPins
                dashboardId={dashboardId}
                panelId={panelId}
                panelTitle={panelName}
                surfaceId={surfaceId}
                store={layoutStore}
                body={body}
                view={pinView(state, tableView)}
              />
            ) : null}
          </div>
        </AnchorLayoutProvider>
      </PanelChrome>
      {pointing !== "select" ? (
        <CanvasLayer
          containerRef={ref}
          surfaceRef={surfaceRef}
          store={layoutStore}
          surfaceId={surfaceId}
          panelId={panelId}
          tool={pointing}
          body={body}
          shownRange={shownWindow}
          wholePanel={tableView && panel.type !== "table"}
        />
      ) : null}
      {askSpot !== null &&
      onCanvas &&
      !layoutSession &&
      dashboardId !== undefined ? (
        <AskPopover
          key={spotKeyOf(askSpot)}
          dashboardId={dashboardId}
          panelTitle={panelName}
          surfaceRef={surfaceRef}
          gate={gateInput}
          spot={askSpot}
          turn={openTurn}
          openChatId={openChat.chatId}
          onChatStarted={openChat.select}
        />
      ) : null}
      {draft?.tool === "comment" &&
      onCanvas &&
      !layoutSession &&
      dashboardId !== undefined ? (
        <CanvasComposer
          key={spotKeyOf(draft)}
          dashboardId={dashboardId}
          panelTitle={panelName}
          draft={draft}
          surfaceRef={surfaceRef}
          gate={gateInput}
        />
      ) : null}
      {selected && !viewing ? (
        <PanelToolbar
          panel={panel}
          permissions={permissions}
          composerRef={composerRef}
          from={rangeFrom}
          to={rangeTo}
          idle={!layoutSession}
          onCanvas={onCanvas}
          menu={panelMenu(true)}
          observationVariables={observationVariables}
        />
      ) : null}
    </div>
  )
})
