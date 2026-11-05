import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react"

import { annotationsFor } from "@/annotations/markers"
import { useDashboardAnnotations, type Annotation } from "@/annotations/queries"
import type { Selections } from "@/api/keys"
import { PanelCommentBadge } from "@/comments/panel-comment-badge"
import { isRenderable, isStaticPanel, preloadRenderer } from "@/panels"
import { createFieldResolver, panelLinks } from "@/panels/field-config"
import { linkScopeFor, PanelBody, StaticPanelBody } from "@/panels/panel-body"
import { panelHasData } from "@/panels/panel-has-data"
import type { SharedCursor } from "@/panels/shared-cursor"
import { reusedFilterOf, reusePanel } from "@/panels/transforms/reuse-panel"
import {
  isDataPanel,
  panelTitle,
  type DataPanel,
  type LayoutPanel,
  type Variable,
} from "@/spec"

import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { panelDatasourceUids } from "./panel-datasources"
import { PanelMenu } from "./panel-menu"
import { stateScope, type PanelSource } from "./panel-source"
import { useAnnotationTagFilter, useSetDashboardSearch } from "./search"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { useSeen } from "./use-seen"
import { UnverifiedLabel } from "./unverified-label"
import { VariableLoadError } from "./variables"

function focusPanel(root: HTMLElement | null) {
  const article = root?.querySelector<HTMLElement>("article")
  if (!article) return
  article.tabIndex = -1
  article.scrollIntoView({ block: "center", behavior: "auto" })
  article.focus({ preventScroll: true })
}

type PanelContainerProps = {
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
  highlighted: boolean
  editingPanel: boolean
  canComment: boolean
  canEdit: boolean
  onToggleResponsive?: (panel: LayoutPanel) => void
  focusOnRender: boolean
  onFocused: () => void
  composerRef: RefObject<HTMLTextAreaElement | null>
  annotations?: readonly Annotation[]
}

export const PanelContainer = memo(function PanelContainer({
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
  highlighted,
  editingPanel,
  canComment,
  canEdit,
  onToggleResponsive,
  focusOnRender,
  onFocused,
  composerRef,
  annotations: givenAnnotations,
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

  const query = usePanelData({
    source,
    panel: dataPanel ?? panel,
    from,
    to,
    timezone,
    vars: selections,
    maxDataPoints: maxDataPointsFor(dataPanel ?? panel, body.width),
    refreshMs,
    enabled: dataPanel !== undefined && supported && seen && selectionsReady,
  })

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
            onToggleResponsive={
              onToggleResponsive && !layoutSession
                ? () => onToggleResponsive(panel)
                : undefined
            }
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
      </PanelChrome>
    </div>
  )
})
