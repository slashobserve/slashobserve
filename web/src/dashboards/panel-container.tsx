import {
  memo,
  useCallback,
  useEffect,
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
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { PanelHealthBadge } from "@/health/panel-health-badge"
import type { PanelHealth } from "@/health/use-dashboard-health"
import { downloadCsv } from "@/inspect/csv"
import type { InspectTarget } from "@/inspect/inspect-pane"
import { savePanelPng } from "@/inspect/png"
import { isRenderable, isStaticPanel, preloadRenderer } from "@/panels"
import { createFieldResolver, panelLinks } from "@/panels/field-config"
import { linkScopeFor, PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import type { SharedCursor } from "@/panels/shared-cursor"
import { prepareFrames, type DatasourceTypeOf } from "@/panels/transforms"
import type { ReusedFilter } from "@/panels/transforms/ad-hoc-filter"
import { reusedFilterOf, reusePanel } from "@/panels/transforms/reuse-panel"
import { useGazetteers } from "@/panels/transforms/use-gazetteers"
import type { WireFrame } from "@/dashboards/queries"
import { useHoveredPanel } from "@/shortcuts/hovered-panel"
import {
  isDataPanel,
  panelTitle,
  resolveDatasourceRef,
  type DataPanel,
  type LayoutPanel,
  type Variable,
} from "@/spec"

import { useCanvasTool } from "./canvas-tools"
import { commentOnPanel } from "./panel-actions"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { panelDatasourceUids } from "./panel-datasources"
import { PanelMenu } from "./panel-menu"
import { PanelToolbar } from "./panel-toolbar"
import {
  panelDataKey,
  panelFingerprint,
  stateScope,
  type PanelSource,
} from "./panel-source"
import { useAnnotationTagFilter, useSetDashboardSearch } from "./search"
import { useSelectionStore } from "./selection-store"
import { useInView } from "./use-in-view"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { UnverifiedLabel } from "./unverified-label"
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
  layoutSession: boolean
  openComments: number
  unverifiedReason: string | undefined
  health?: PanelHealth
  highlighted: boolean
  selected?: boolean
  editingPanel: boolean
  canComment: boolean
  canEdit: boolean
  onToggleResponsive?: (panel: LayoutPanel) => void
  focusOnRender: boolean
  onFocused: () => void
  composerRef: RefObject<HTMLTextAreaElement | null>
  annotations?: readonly Annotation[]
  viewing?: boolean
  inspected?: boolean
  onInspectTarget?: (target: InspectTarget | null) => void
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
  layoutSession,
  openComments,
  unverifiedReason,
  health,
  highlighted,
  selected = false,
  editingPanel,
  canComment,
  canEdit,
  onToggleResponsive,
  focusOnRender,
  onFocused,
  composerRef,
  annotations: givenAnnotations,
  viewing = false,
  inspected = false,
  onInspectTarget,
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
  const { seen, inView } = useInView(ref)
  const [tableView, setTableView] = useState(false)
  const setSearch = useSetDashboardSearch()
  const commentTool = useCanvasTool((state) => state.tool === "comment")
  const body = panelBodySize(Math.floor(width), Math.floor(height))
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
      if (sourceKind === "build") return
      setSearch((state) => ({
        ...state,
        from: String(Math.round(fromMs)),
        to: String(Math.round(toMs)),
      }))
    },
    [setSearch, sourceKind]
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
    () => useHoveredPanel.getState().enter(panelId),
    [panelId]
  )
  const onPointerLeave = useCallback(
    () => useHoveredPanel.getState().leave(panelId),
    [panelId]
  )
  useEffect(() => () => useHoveredPanel.getState().leave(panelId), [panelId])

  const permissions = useMemo(
    () => ({ comment: canComment, edit: canEdit }),
    [canComment, canEdit]
  )
  const selectPanel = useMemo(
    () =>
      dashboardId === undefined
        ? undefined
        : () => useSelectionStore.getState().select(dashboardId, panelId),
    [dashboardId, panelId]
  )
  const onPointerDownCapture = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || viewing) return
    const target = event.target as Element
    if (target.closest(interactiveTarget)) return
    if (commentTool) {
      if (layoutSession) return
      event.preventDefault()
      event.stopPropagation()
      useCanvasTool.getState().setTool("select")
      commentOnPanel(setSearch, panelId)
      requestAnimationFrame(() => composerRef.current?.focus())
      return
    }
    if (!target.closest("[data-panel-body]")) selectPanel?.()
  }

  const data = query.data
  const onDownloadCsv = useMemo(
    () =>
      data && state === "ready"
        ? () => downloadCsv(prepare(data.frames), panelName)
        : undefined,
    [data, state, prepare, panelName]
  )
  const onSavePng = useMemo(
    () =>
      state === "ready"
        ? () => {
            const element = bodyRef.current
            if (!element) return
            savePanelPng(element, panelName).catch(() =>
              toast.error("Could not save the panel as PNG")
            )
          }
        : undefined,
    [state, panelName]
  )

  const panelMenu = (inToolbar: boolean) => (
    <PanelMenu
      panel={panel}
      tableView={tableView}
      onToggleTableView={() => setTableView((current) => !current)}
      idle={!layoutSession}
      viewing={viewing}
      onDownloadCsv={onDownloadCsv}
      onSavePng={onSavePng}
      canComment={canComment}
      canEdit={canEdit}
      onToggleResponsive={
        onToggleResponsive && !layoutSession
          ? () => onToggleResponsive(panel)
          : undefined
      }
      composerRef={composerRef}
      inToolbar={inToolbar}
    />
  )

  return (
    <div
      ref={ref}
      className="relative size-full"
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
      </PanelChrome>
      {selected && !viewing ? (
        <PanelToolbar
          panel={panel}
          permissions={permissions}
          composerRef={composerRef}
          from={from}
          to={to}
          idle={!layoutSession}
          menu={panelMenu(true)}
        />
      ) : null}
    </div>
  )
})
