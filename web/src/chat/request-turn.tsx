import { useQuery } from "@tanstack/react-query"
import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { useEffect, useState, type ReactNode } from "react"

import { ApiError } from "@/api/errors"
import { AgentMark } from "@/components/agent-mark"
import {
  AnswerCard,
  reviseFailureText,
  type RevisionAsked,
} from "@/conversation/answer-card"
import {
  defaultDelivery,
  deliveryGate,
  reviseBlock,
  ungated,
  type Place,
} from "@/conversation/delivery"
import {
  planRevise,
  threadTargetOf,
  type Subject,
} from "@/conversation/send-plan"
import { useDeliver } from "@/conversation/use-composer"
import { StatusIcon } from "@/components/status-label"
import {
  buildStatusText,
  planChangeText,
  type BuildProgress,
} from "@/observe/build"
import { costText, shortRequestId, stepSummary } from "@/observe/conversation"
import {
  isTerminal,
  observeRequestQuery,
  type ObserveRequest,
  stopsPolling,
  useAgentInfo,
  usePollFailures,
} from "@/observe/queries"
import { CancelRequestButton } from "@/observe/request-actions"
import { requestOrigin, type OriginFields } from "@/observe/request-origin"
import { runWords } from "@/observe/run-words"
import { useSettledRefresh } from "@/observe/use-settled-refresh"

import { FindingCard } from "@/findings/finding-card"
import type { Finding } from "@/findings/queries"

import { AskOnDashboard } from "./ask-on-dashboard"
import { BuildCard } from "./build-card"
import { PromptBubble } from "./prompt-bubble"
import { ResultCard } from "./result-card"
import { RunningFor } from "./run-duration"
import { StepTimeline } from "./step-timeline"
import {
  RetryButton,
  Stalled,
  TurnFailure,
  useOpenNewRun,
  type RunOpens,
} from "./turn-failure"
import {
  mayHaveFinding,
  noFindingText,
  resultFinishedAt,
  showsWorkingRow,
  tookText,
} from "./turn-model"

const reconnectingAfterFailures = 2

export type TurnSummary = OriginFields & {
  id: string
  prompt: string
  redacted: boolean
  investigation: boolean
  status: "queued" | "running" | "succeeded" | "failed"
  state: ObserveRequest["state"]
  result_kind: ObserveRequest["result_kind"]
  allowed_actions: ObserveRequest["allowed_actions"]
  dashboard_id: string | null
  result_version: number | null
  error: string | null
  panel_id: string | null
  requested_by: { id: string; name: string }
  created_at: string
  started_at: string | null
  finished_at: string | null
  failure_code: string | null
  build: BuildProgress | null
  build_saved_at: string | null
  continues_id: string | null
  continued_by: string | null
  retried_by: string | null
  continue_note: string | null
  from?: string | null
  to?: string | null
}

type Props = {
  summary: TurnSummary
  dashboardTitle?: string
  loadSteps: boolean
  onRetried: (request: ObserveRequest) => void
  buildOnScreen?: boolean
  opensRunsIn?: RunOpens
  latest?: boolean
  answerActions?: ReactNode
  about?: Subject | null
}

export function ChatAnswer({
  request,
  finding,
  own,
  about,
  onRevised,
  showToEveryone,
  reviseNote,
  actions,
}: {
  request: ObserveRequest
  finding: Finding
  own: boolean
  about: Subject | null
  onRevised: (next: ObserveRequest) => Promise<void>
  showToEveryone?: boolean
  reviseNote?: string
  actions?: ReactNode
}) {
  const dashboardId = request.dashboard_id
  const revised: Subject | null =
    dashboardId === null ? null : { kind: "dashboard", id: dashboardId }
  const subject = revised ?? about
  const asked: Subject | null =
    request.subject_observation_id === null
      ? subject
      : { kind: "observation", id: request.subject_observation_id }
  const { deliver, pending } = useDeliver(subject)
  const agent = useAgentInfo()
  const mayRevise = own && !request.read_only && revised !== null
  const place: Place = { kind: "chat", chatId: request.chat_id }
  const gate = deliveryGate(
    { ...ungated, agentAvailable: agent.data?.available },
    defaultDelivery(place, "")
  )

  async function revise(): Promise<RevisionAsked> {
    const plan = planRevise(place, revised, finding.id, crypto.randomUUID())
    if ("error" in plan) return plan.error
    try {
      const sent = await deliver(plan)
      if (sent.via === "chat") await onRevised(sent.request)
      return true
    } catch (error) {
      if (error instanceof ApiError && error.code === "already_applied") {
        return true
      }
      return reviseFailureText(error)
    }
  }

  return (
    <AnswerCard
      finding={finding}
      requestId={request.id}
      canSave={own}
      revise={
        mayRevise
          ? {
              onRevise: revise,
              pending,
              blocked: reviseBlock(gate, true),
              note: reviseNote,
              dashboardId: revised.id,
            }
          : undefined
      }
      chatId={request.chat_id ?? undefined}
      actions={actions}
      menu={{
        shareTarget: own && asked !== null ? threadTargetOf(asked) : null,
        sharedThreadId: request.shared_thread_id,
        showToEveryone,
        subjectObservationId: request.subject_observation_id,
      }}
    />
  )
}

function isActive(status: TurnSummary["status"]): boolean {
  return status === "queued" || status === "running"
}

export function opensBuildPage({
  request,
  own,
  onChatPage,
}: {
  request: OriginFields & Pick<TurnSummary, "status" | "dashboard_id" | "build">
  own: boolean
  onChatPage: boolean
}): boolean {
  return (
    onChatPage &&
    own &&
    requestOrigin(request) === "observe" &&
    isActive(request.status) &&
    request.dashboard_id === null &&
    request.build !== null
  )
}

export function workingText(
  request: Pick<TurnSummary, "dashboard_id" | "build">,
  dashboardTitle: string | undefined
): string {
  if (dashboardTitle !== undefined) return `Revising ${dashboardTitle}`
  if (request.dashboard_id !== null) return "Revising the dashboard"
  return request.build === null
    ? "Working on your request"
    : "Building the dashboard"
}

function Sender({ summary }: { summary: TurnSummary }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const own = summary.requested_by.id === principal.id
  const fromDashboard =
    requestOrigin(summary) === "comment" || summary.panel_id !== null
  if (own && !fromDashboard) return null
  return (
    <p className="self-end text-xs text-muted-foreground">
      {own
        ? "Sent from the dashboard"
        : `Sent by ${summary.requested_by.name}${fromDashboard ? " from the dashboard" : ""}`}
    </p>
  )
}

function ContinuedFrom({ summary }: { summary: TurnSummary }) {
  if (summary.continues_id === null) return null
  return (
    <>
      <p className="self-end text-xs text-muted-foreground">
        Continues an earlier run
      </p>
      {summary.continue_note !== null ? (
        <p className="max-w-[80%] self-end text-xs wrap-anywhere whitespace-pre-wrap text-muted-foreground">
          {summary.continue_note}
        </p>
      ) : null}
    </>
  )
}

export function RequestTurn({
  summary,
  dashboardTitle,
  loadSteps,
  onRetried,
  buildOnScreen = false,
  opensRunsIn = "chat",
  latest = true,
  answerActions,
  about = null,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const [detailsOpen, setDetailsOpen] = useState(false)
  const query = useQuery({
    ...observeRequestQuery(summary.id),
    enabled:
      isActive(summary.status) ||
      loadSteps ||
      detailsOpen ||
      mayHaveFinding(summary),
  })
  const live = query.data
  const settled = !isActive(summary.status)
  const showLive = live !== undefined && !(settled && !isTerminal(live))
  const request: TurnSummary = showLive ? live : summary
  const readsData = request.kind === "investigate"
  const words = runWords(request)
  const investigates = request.investigation
  const detail = showLive ? live : undefined
  const build = request.build
  const answers = mayHaveFinding(request)
  const openNewRun = useOpenNewRun(request, onRetried, opensRunsIn)
  const mayReadBuild =
    request.requested_by.id === principal.id || live?.read_only === true
  const active = isActive(request.status)
  const stalled = active && stopsPolling(query.error)
  const pollFailures = usePollFailures(summary.id)
  const reconnecting =
    active && !stalled && pollFailures >= reconnectingAfterFailures
  const steps = detail?.steps ?? []
  const planChange = planChangeText(steps)
  const savedBuild = request.result_kind === "partial_version"
  const inPane = opensRunsIn === "build"
  const navigate = useNavigate()
  const opensBuild = opensBuildPage({
    request,
    own: request.requested_by.id === principal.id,
    onChatPage: opensRunsIn === "chat",
  })
  useEffect(() => {
    if (!opensBuild) return
    void navigate({
      to: "/b/$requestId",
      params: { requestId: summary.id },
      replace: true,
    })
  }, [opensBuild, navigate, summary.id])

  useSettledRefresh({
    active,
    status: request.status,
    dashboardId: request.dashboard_id,
  })

  return (
    <section
      id={`request-${summary.id}`}
      aria-label={`Request ${shortRequestId(summary.id)}`}
      aria-busy={active}
      className="flex scroll-mt-4 flex-col gap-[18px]"
    >
      <div className="flex flex-col gap-1.5">
        <Sender summary={request} />
        <ContinuedFrom summary={request} />
        <PromptBubble prompt={words} />
      </div>
      <div className="flex gap-3">
        <AgentMark variant="tile" />
        <div className="flex min-w-0 flex-1 flex-col gap-3 pt-[3px]">
          {active ? (
            <div className="flex flex-col gap-2.5">
              {build !== null && !readsData ? (
                <BuildCard
                  request={detail ?? request}
                  build={build}
                  mayReadBuild={mayReadBuild}
                  onContinued={(next) => void openNewRun(next)}
                  compact={buildOnScreen}
                />
              ) : (
                <span className="flex items-center gap-2 text-[13px]">
                  <StatusIcon tone="running" />
                  {detail !== undefined &&
                  (detail.queue_position !== null ||
                    detail.state === "canceling")
                    ? buildStatusText({
                        status: detail.status,
                        build: null,
                        failureCode: detail.failure_code,
                        queuePosition: detail.queue_position,
                        canceling: detail.state === "canceling",
                      })
                    : readsData
                      ? "Investigating"
                      : workingText(request, dashboardTitle)}
                </span>
              )}
              {detail?.can_cancel ? (
                <div className="self-start">
                  <CancelRequestButton request={detail} />
                </div>
              ) : null}
              <StepTimeline
                steps={steps}
                working={showsWorkingRow(request, stalled)}
                reconnecting={reconnecting}
                liveText={detail?.live_text}
                requestId={summary.id}
                pane={inPane}
                withMessages={words !== null}
              />
              {request.status === "running" ? (
                <RunningFor startedAt={request.started_at} />
              ) : null}
              {stalled ? (
                <Stalled
                  message={query.error?.message ?? ""}
                  retrying={query.isFetching}
                  onRetry={() => void query.refetch()}
                />
              ) : null}
            </div>
          ) : (
            <details
              onToggle={(event) => setDetailsOpen(event.currentTarget.open)}
            >
              <summary className="cursor-pointer text-[13px] text-muted-foreground">
                {detail === undefined
                  ? "Show steps"
                  : stepSummary(
                      detail.steps,
                      tookText(request.started_at, request.finished_at),
                      costText(detail.usage?.cost_usd)
                    )}
              </summary>
              <div className="flex flex-col gap-2.5 pt-2.5 pl-0.5">
                {detail !== undefined ? (
                  <StepTimeline
                    steps={steps}
                    working={false}
                    reconnecting={false}
                    requestId={summary.id}
                    pane={inPane}
                    withMessages={words !== null}
                  />
                ) : query.isError ? (
                  <div className="flex flex-wrap items-center gap-3 text-[13px] text-muted-foreground">
                    <span>Steps could not be loaded.</span>
                    <RetryButton
                      busy={query.isFetching}
                      onClick={() => void query.refetch()}
                    >
                      Retry
                    </RetryButton>
                  </div>
                ) : (
                  <p className="text-[13px] text-muted-foreground">
                    Loading steps...
                  </p>
                )}
              </div>
            </details>
          )}
          {!active && planChange !== null ? (
            <p className="text-[13px] text-muted-foreground">{planChange}</p>
          ) : null}
          {savedBuild ? (
            <p className="text-[13px]">
              {request.requested_by.id === principal.id
                ? "You"
                : request.requested_by.name}{" "}
              saved what was built as version {request.result_version}.
            </p>
          ) : null}
          {answers && request.status === "succeeded" ? (
            detail?.finding != null ? (
              <>
                {!investigates ? (
                  <ChatAnswer
                    request={detail}
                    finding={detail.finding}
                    own={request.requested_by.id === principal.id}
                    about={about}
                    onRevised={openNewRun}
                    actions={answerActions}
                  />
                ) : (
                  <FindingCard
                    finding={detail.finding}
                    showOpenLink
                    chatId={detail.chat_id ?? undefined}
                    actions={answerActions}
                  />
                )}
                {detail.finding.ask_on !== null && words !== null ? (
                  <AskOnDashboard
                    askOn={detail.finding.ask_on}
                    prompt={words}
                  />
                ) : null}
              </>
            ) : detail !== undefined &&
              (request.result_kind === "none" || !query.isFetching) ? (
              <p className="text-[13px] text-muted-foreground">
                {noFindingText(detail)}
              </p>
            ) : null
          ) : null}
          {!answers &&
          (request.status === "succeeded" || savedBuild) &&
          request.result_version !== null &&
          request.dashboard_id !== null ? (
            <ResultCard
              dashboardId={request.dashboard_id}
              version={request.result_version}
              finishedAt={resultFinishedAt(request)}
              eager={loadSteps}
              partial={
                savedBuild && words !== null ? { prompt: words } : undefined
              }
              addMissing={latest}
            />
          ) : null}
          {request.status === "failed" && !savedBuild ? (
            <TurnFailure
              request={request}
              onRetried={onRetried}
              opensRunsIn={opensRunsIn}
            />
          ) : null}
          {request.status === "failed" && !savedBuild && build !== null ? (
            <BuildCard
              request={request}
              build={build}
              mayReadBuild={mayReadBuild}
              onContinued={(next) => void openNewRun(next)}
              compact={buildOnScreen}
            />
          ) : null}
        </div>
      </div>
    </section>
  )
}
