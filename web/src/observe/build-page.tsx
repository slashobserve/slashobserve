import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { Link, useNavigate, useRouteContext } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { StatusIcon } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { DashboardGrid } from "@/dashboards/dashboard-grid"
import { DashboardTopRow } from "@/dashboards/dashboard-top-row"
import { PanelContainer } from "@/dashboards/panel-container"
import type { PanelSource } from "@/dashboards/panel-source"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { useVariables } from "@/dashboards/use-variables"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { createSharedCursor } from "@/panels/shared-cursor"
import { ThemeSwitch } from "@/shell/theme-switch"
import { isDataPanel, sourcePanelOf, type Dashboard } from "@/spec"

import {
  buildQuery,
  buildStatusText,
  canSaveBuild,
  newPanelIds,
  saveBuildErrorText,
  useSaveBuild,
} from "./build"
import { buildPageNotice, buildPageTitle } from "./build-page-model"
import { observeRequestQuery, sentence, type ObserveRequest } from "./queries"
import { CancelRequestButton, ContinueBuildButton } from "./request-actions"

const noPanels: ReadonlySet<string> = new Set()
const noUrlVars: Record<string, string[]> = {}
const fadeIn =
  "motion-safe:animate-in motion-safe:fade-in-0 motion-safe:duration-300"

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
      renderPanel={(item, size) => (
        <div className={cn("size-full", fresh.has(item.panel.id) && fadeIn)}>
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
        </div>
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

function dashboardLink(request: ObserveRequest, label: string) {
  if (request.dashboard_id === null) return null
  return (
    <Link
      to="/dashboards/$dashboardId"
      params={{ dashboardId: request.dashboard_id }}
      className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {label}
    </Link>
  )
}

function StoppedStrip({ request }: { request: ObserveRequest }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const navigate = useNavigate()
  const save = useSaveBuild(request.id)
  const isRequester = request.requested_by.id === principal.id
  const mayRetry = canSaveBuild({
    status: request.status,
    build: request.build,
    buildSavedAt: request.build_saved_at,
    isRequester,
  })
  const error = save.error
  const code = error instanceof ApiError ? error.code : ""
  return (
    <div className="flex flex-col gap-2 pr-2.5 pb-2.5 pl-4 text-[13px]">
      {request.error !== null ? (
        <p className="text-muted-foreground">{sentence(request.error)}</p>
      ) : null}
      {request.build_saved_at !== null && request.result_version !== null ? (
        <p className="flex flex-wrap items-center gap-x-2">
          <span>
            You saved what was built as version {request.result_version}.
          </span>
          {dashboardLink(request, "Open the dashboard")}
        </p>
      ) : null}
      {request.continued_by !== null ? (
        <p>
          <Link
            to="/builds/$requestId"
            params={{ requestId: request.continued_by }}
            className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Continued in a new run
          </Link>
        </p>
      ) : null}
      {mayRetry && request.continued_by === null ? (
        <div className="flex flex-wrap items-start gap-3">
          <div className="flex flex-col items-start gap-1">
            <Button
              disabled={save.isPending}
              aria-busy={save.isPending}
              onClick={() =>
                save.mutate(undefined, {
                  onSuccess: (saved) =>
                    void navigate({
                      to: "/dashboards/$dashboardId",
                      params: { dashboardId: saved.dashboard_id },
                    }),
                })
              }
            >
              {save.isPending ? "Saving" : "Save what was built"}
            </Button>
            {request.dashboard_id === null ? (
              <p className="text-muted-foreground">
                Saves to your My Dashboards.
              </p>
            ) : null}
          </div>
          <ContinueBuildButton
            request={request}
            isRequester={isRequester}
            onContinued={(next) =>
              void navigate({
                to: "/builds/$requestId",
                params: { requestId: next.id },
              })
            }
          />
          {error !== null ? (
            <p role="alert" className="flex flex-wrap items-center gap-x-2">
              <span>{saveBuildErrorText(code, error.message)}</span>
              {code === "version_conflict"
                ? dashboardLink(request, "Open the dashboard")
                : null}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function BuildPage({ requestId }: { requestId: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
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

  const succeededId =
    request?.status === "succeeded" ? request.dashboard_id : null
  useEffect(() => {
    if (succeededId === null) return
    void navigate({
      to: "/dashboards/$dashboardId",
      params: { dashboardId: succeededId },
      replace: true,
    })
  }, [succeededId, navigate])

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
  const status = request
    ? buildStatusText({
        status: request.status,
        build: request.build,
        failureCode: request.failure_code,
        queuePosition: request.queue_position,
        canceling: request.cancel_requested_at !== null,
        continued: request.continued_by !== null,
      })
    : ""

  let body
  if (notice !== undefined) {
    body = <Notice>{notice}</Notice>
  } else if (loaded && spec) {
    body = (
      <main className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4">
        <BuildGrid
          requestId={requestId}
          saves={loaded.saves}
          spec={spec}
          fresh={seen.fresh}
        />
      </main>
    )
  } else {
    body = <main className="min-h-0 flex-1" aria-busy={active} />
  }

  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <DashboardTopRow
        title={knownTitle ?? (dashboardId === null ? "New dashboard" : title)}
        madeBy={null}
        versionControl={null}
        controls={[
          <p
            key="status"
            aria-live="polite"
            className="flex items-center gap-2 text-[13px]"
          >
            {active ? <StatusIcon tone="running" /> : null}
            {request?.status === "failed" ? (
              <StatusIcon tone="critical" />
            ) : null}
            {status}
          </p>,
        ]}
        editing={null}
        actions={[
          request ? (
            <CancelRequestButton key="cancel" request={request} />
          ) : null,
          <Link
            key="chat"
            to="/observe/$requestId"
            params={{ requestId }}
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            Back to chat
          </Link>,
          <ThemeSwitch key="theme" />,
        ]}
      />
      {request?.status === "failed" ? <StoppedStrip request={request} /> : null}
      {body}
    </div>
  )
}
