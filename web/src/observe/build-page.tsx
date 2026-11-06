import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { useChatPaneLayout } from "@/activity/activity-store"
import { ChatPane } from "@/chat/chat-pane"
import { DashboardGrid } from "@/dashboards/dashboard-grid"
import { pageClass } from "@/dashboards/dashboard-page-skeleton"
import { DashboardTopRow } from "@/dashboards/dashboard-top-row"
import { PanelContainer } from "@/dashboards/panel-container"
import { mixesDatasources } from "@/dashboards/panel-datasources"
import type { PanelSource } from "@/dashboards/panel-source"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { useVariables } from "@/dashboards/use-variables"
import { useDocumentTitle } from "@/lib/document-title"
import { createSharedCursor } from "@/panels/shared-cursor"
import { Sidebar } from "@/shell/sidebar"
import { isDataPanel, sourcePanelOf, type Dashboard } from "@/spec"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"

import {
  answeredRequestId,
  buildQuery,
  newPanelIds,
  savedDashboardId,
} from "./build"
import {
  buildBodyState,
  buildPageNotice,
  buildPageTitle,
} from "./build-page-model"
import { BuildWaiting, NothingSaved } from "./build-waiting"
import { observeRequestQuery } from "./queries"

const noPanels: ReadonlySet<string> = new Set()
const noUrlVars: Record<string, string[]> = {}

function ignoreFocus() {}

function BuildGrid({
  requestId,
  saves,
  spec,
  fresh,
}: {
  requestId: string
  saves: number
  spec: Dashboard
  fresh: ReadonlySet<string>
}) {
  const source = useMemo<PanelSource>(
    () => ({ kind: "build", requestId, saves }),
    [requestId, saves]
  )
  const variables = useMemo(() => spec.variables ?? [], [spec])
  const { selections, values, ready, blocking } = useVariables({
    source,
    variables,
    from: spec.time.from,
    to: spec.time.to,
    timezone: spec.timezone,
    urlVars: noUrlVars,
  })
  const mixedDatasources = useMemo(
    () => mixesDatasources(spec, selections),
    [spec, selections]
  )
  const [cursor] = useState(createSharedCursor)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const retryBlocking = useRef<(() => void) | undefined>(undefined)
  useEffect(() => {
    retryBlocking.current = blocking?.retry
  })
  const onRetryBlocked = useCallback(() => retryBlocking.current?.(), [])
  const blockedBy = blocking
    ? (blocking.variable.label ?? blocking.variable.name)
    : undefined

  return (
    <DashboardGrid
      dashboardId={`build:${requestId}`}
      spec={spec}
      selections={selections}
      values={values}
      newPanelIds={fresh}
      renderPanel={(item, size) => (
        <PanelContainer
          source={source}
          cursor={cursor}
          panel={item.panel}
          reused={
            isDataPanel(item.panel)
              ? sourcePanelOf(spec, item.panel)
              : undefined
          }
          variables={variables}
          title={item.title}
          repeatOf={item.key === item.panel.id ? undefined : item.panel.id}
          width={size.width}
          height={size.height}
          from={spec.time.from}
          to={spec.time.to}
          timezone={spec.timezone}
          rangeFrom={spec.time.from}
          rangeTo={spec.time.to}
          selections={item.selections}
          selectionsReady={ready}
          blockedBy={blockedBy}
          onRetryBlocked={onRetryBlocked}
          refreshMs={null}
          mixedDatasources={mixedDatasources}
          editable={false}
          layoutSession={false}
          openComments={0}
          unverifiedReason={undefined}
          highlighted={false}
          editingPanel={false}
          canComment={false}
          canEdit={false}
          focusOnRender={false}
          onFocused={ignoreFocus}
          composerRef={composerRef}
        />
      )}
    />
  )
}

function Notice({ children }: { children: string }) {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="text-sm text-muted-foreground">{children}</p>
    </main>
  )
}

export function BuildPage({ requestId }: { requestId: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { stacked } = useChatPaneLayout()
  const requestQuery = useQuery(observeRequestQuery(requestId))
  const request = requestQuery.data
  const dashboardId = request?.dashboard_id ?? null
  const dashboard = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: dashboardId !== null,
  })
  const saves = request?.build?.saves
  const buildResult = useQuery({
    ...buildQuery(requestId, saves ?? 0),
    enabled: saves !== undefined,
    placeholderData: keepPreviousData,
  })
  const loaded = buildResult.data
  const spec = useMemo(
    () => (loaded ? asDashboardSpec(loaded.spec) : undefined),
    [loaded]
  )

  const [seen, setSeen] = useState<{
    saves: number | undefined
    spec: Dashboard | undefined
    fresh: ReadonlySet<string>
  }>({ saves: undefined, spec: undefined, fresh: noPanels })
  if (loaded && spec && loaded.saves !== seen.saves) {
    setSeen({
      saves: loaded.saves,
      spec,
      fresh: seen.spec ? newPanelIds(seen.spec, spec) : noPanels,
    })
  }

  const savedId = request === undefined ? null : savedDashboardId(request)
  useEffect(() => {
    if (savedId === null) return
    void navigate({
      ...dashboardLink(savedId),
      search: { chat: requestId },
      replace: true,
      viewTransition: true,
    })
  }, [savedId, navigate, requestId])

  const answeredId = request === undefined ? null : answeredRequestId(request)
  useEffect(() => {
    if (answeredId === null) return
    void navigate({
      to: "/c/$requestId",
      params: { requestId: answeredId },
      replace: true,
    })
  }, [answeredId, navigate])

  const active = request?.status === "queued" || request?.status === "running"
  const knownTitle = buildPageTitle({
    specTitle: spec?.title,
    buildTitle: request?.build?.title,
    dashboardTitle: dashboard.data?.dashboard.title,
  })
  const title = knownTitle ?? "the dashboard"
  useDocumentTitle(
    active ? `Building ${title} - SlashObserve` : `${title} - SlashObserve`
  )

  const notice = buildPageNotice({
    requestError: requestQuery.error,
    hasRequest: request !== undefined,
    buildError: buildResult.error,
  })
  const bodyState = buildBodyState({
    failed: request?.status === "failed",
    savedPanels: spec?.panels.length ?? 0,
  })
  const body =
    notice !== undefined ? (
      <Notice>{notice}</Notice>
    ) : (
      <main
        className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4"
        aria-busy={bodyState === "waiting" && active}
      >
        {bodyState === "grid" && loaded && spec ? (
          <BuildGrid
            requestId={requestId}
            saves={loaded.saves}
            spec={spec}
            fresh={seen.fresh}
          />
        ) : bodyState === "nothing-saved" ? (
          <NothingSaved />
        ) : (
          <BuildWaiting />
        )}
      </main>
    )

  const topRow = (
    <DashboardTopRow
      title={knownTitle ?? (dashboardId === null ? "New dashboard" : title)}
      madeBy={null}
      versionControl={null}
      controls={[]}
      editing={null}
      actions={[]}
    />
  )

  return (
    <div className="flex min-h-svh bg-chrome">
      <Sidebar rail />
      <div className={cn(pageClass(stacked), "min-w-0 flex-1")}>
        <div className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
          {topRow}
          {body}
        </div>
        <ChatPane requestId={requestId} />
      </div>
    </div>
  )
}
