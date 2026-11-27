import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState } from "react"

import { ApiError, sentence } from "@/api/errors"
import { ScrollPane } from "@/components/scroll-pane"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { latestDashboardQuery } from "@/dashboards/queries"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { orderTurns, runningOwnTurnId } from "@/observe/conversation"
import {
  useAgentInfo,
  useObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { requestOrigin } from "@/observe/request-origin"
import { privateChatText, runWords } from "@/observe/run-words"
import { revisionsQuery } from "@/revisions/queries"

import { Composer } from "@/conversation/composer"
import { ungated } from "@/conversation/delivery"
import type { Opens, Subject } from "@/conversation/send-plan"
import { useComposer } from "@/conversation/use-composer"

import { ExampleChips } from "@/home/task-card"

import { AskAgainButton, useAskInChat } from "./ask-again"
import { useChat } from "./chats-queries"
import { useOpenSent } from "./open-sent"
import { useChatScroll } from "./chat-scroll"
import { RequestNotFound } from "./request-not-found"
import { RequestTurn, type TurnSummary } from "./request-turn"
import { offersFollowUps } from "./turn-model"
import { RetryButton } from "./turn-failure"
import {
  panelChips,
  panelTarget,
  useFocusOnPanel,
  type ChatPanel,
} from "./panel-target"

const stepsLoadedTurns = 3
const titlePromptLength = 60

function isActiveStatus(status: TurnSummary["status"]): boolean {
  return status === "queued" || status === "running"
}

function TurnSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-[18px]">
      <Skeleton className="h-11 w-2/3 self-end rounded-xl" />
      <div className="flex gap-3">
        <Skeleton className="size-[26px] flex-none rounded-[7px]" />
        <Skeleton className="h-[172px] flex-1 rounded-card" />
      </div>
    </div>
  )
}

function ConversationSkeleton() {
  return (
    <div className="mx-auto w-full max-w-[760px] pt-2">
      <TurnSkeleton />
    </div>
  )
}

function LoadError({
  message,
  busy,
  onRetry,
}: {
  message: string
  busy: boolean
  onRetry: () => void
}) {
  return (
    <div className="mx-auto w-full max-w-[760px]">
      <Alert variant="destructive">
        <AlertTitle>Could not load this request</AlertTitle>
        <AlertDescription>
          <p>{sentence(message)}</p>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <RetryButton busy={busy} onClick={onRetry}>
              Retry
            </RetryButton>
            <Link to="/ask" className="underline underline-offset-4">
              Back to Observe
            </Link>
          </div>
        </AlertDescription>
      </Alert>
    </div>
  )
}

type Turn = { id: string; summary: TurnSummary }

export type ConversationLayout = "page" | "pane"

function PageTitle({ title }: { title: string }) {
  useDocumentTitle(title)
  return null
}

function PageHeading({ heading }: { heading: string }) {
  const title = useRef<HTMLHeadingElement>(null)
  useEffect(() => {
    title.current?.focus({ preventScroll: true })
  }, [])
  return (
    <h1 ref={title} tabIndex={-1} className="sr-only outline-none">
      {heading}
    </h1>
  )
}

function PaneHeading({ requestId }: { requestId: string }) {
  return (
    <div className="flex w-full items-center justify-between gap-2 pb-2">
      <h2 className="text-sm font-semibold">Chat</h2>
      <Link
        to="/c/$requestId"
        params={{ requestId }}
        className="rounded-sm text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        Open the chat page
      </Link>
    </div>
  )
}

function ConversationComposer({
  chatId,
  subject,
  dashboardTitle,
  panel,
  disabled,
  runningRequestId,
  onSent,
}: {
  chatId: string | null
  subject: Subject | null
  dashboardTitle: string | undefined
  panel: ChatPanel | null
  disabled: boolean
  runningRequestId: string | undefined
  onSent: (request: ObserveRequest, opens: Opens) => Promise<void>
}) {
  const model = useComposer({
    subject,
    place: { kind: "chat", chatId },
    target: panelTarget(panel),
    gate: ungated,
    runningRequestId,
    onSent: (sent) =>
      sent.via === "chat" ? onSent(sent.request, sent.opens) : undefined,
  })
  useFocusOnPanel(panel, model.field)
  return (
    <Composer
      model={model}
      layout="page"
      rows={2}
      disabled={disabled}
      contextChips={[
        ...(dashboardTitle === undefined
          ? []
          : [{ id: "dashboard", label: dashboardTitle }]),
        ...panelChips(panel),
      ]}
      placeholder={
        subject?.kind === "dashboard"
          ? `Ask about ${dashboardTitle ?? "this dashboard"}`
          : subject?.kind === "observation"
            ? "Ask about this observation"
            : "Ask a follow-up, or describe a dashboard to build"
      }
    />
  )
}

type ViewProps = {
  requestId: string
  layout: ConversationLayout
  chatId: string | null
  subject: Subject | null
  dashboardTitle: string | undefined
  turns: Turn[]
  ready: boolean
  historyFailed: boolean
  retryHistory: () => void
  historyBusy: boolean
  readOnly: boolean
  panel: ChatPanel | null
  onSent: (request: ObserveRequest) => void
}

function ConversationView({
  requestId,
  layout,
  chatId,
  subject,
  dashboardTitle,
  turns,
  ready,
  historyFailed,
  retryHistory,
  historyBusy,
  readOnly,
  panel,
  onSent,
}: ViewProps) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const agent = useAgentInfo()
  const list = useRef<HTMLDivElement>(null)
  const pane = layout === "pane"
  const [initialTarget, setInitialTarget] = useState<string | null>(null)
  if (ready && initialTarget === null) {
    setInitialTarget(turns.at(-1)?.id === requestId ? "" : requestId)
  }
  const pinToEnd = useChatScroll(list, initialTarget)

  const first = turns.at(0)
  const heading =
    dashboardTitle ??
    (first === undefined ? "" : (runWords(first.summary) ?? privateChatText))
  const active = turns.some((turn) => isActiveStatus(turn.summary.status))
  const clipped =
    [...heading].length > titlePromptLength
      ? `${[...heading].slice(0, titlePromptLength).join("")}...`
      : heading
  const pageTitle = active
    ? "Observing - SlashObserve"
    : `${clipped} - SlashObserve`
  const runningRequestId = runningOwnTurnId(
    turns.map((turn) => turn.summary),
    principal.id
  )

  function appended(request: ObserveRequest) {
    pinToEnd()
    onSent(request)
  }

  const openSent = useOpenSent(appended)

  function focusList() {
    list.current?.focus({ preventScroll: true })
  }

  const followUp = useAskInChat({
    subject,
    chatId: chatId ?? undefined,
    onSent: openSent,
    afterSent: focusList,
  })

  const agentOff = agent.data?.available === false
  const hasDashboard = subject?.kind === "dashboard"
  const latestTurn = turns.at(-1)
  const waitingForAnswer =
    !hasDashboard &&
    latestTurn !== undefined &&
    isActiveStatus(latestTurn.summary.status)
  const loadFrom = Math.max(0, turns.length - stepsLoadedTurns)
  const suggestsFollowUps =
    pane &&
    !readOnly &&
    !agentOff &&
    offersFollowUps({
      turnCount: turns.length,
      latest: latestTurn?.summary,
      viewerId: principal.id,
      aboutDashboard: hasDashboard,
    })

  const transcript = (
    <>
      {pane ? (
        <PaneHeading requestId={requestId} />
      ) : (
        <>
          <PageTitle title={pageTitle} />
          <PageHeading heading={heading} />
        </>
      )}
      <div
        ref={list}
        role="region"
        aria-label="Conversation"
        tabIndex={-1}
        className={cn(
          "flex w-full flex-1 flex-col gap-[18px] pt-2 pb-6 outline-none",
          !pane && "max-w-[760px]"
        )}
      >
        {historyFailed ? (
          <div className="flex flex-wrap items-center gap-3 text-[13px] text-muted-foreground">
            <span>Earlier messages could not be loaded.</span>
            <RetryButton busy={historyBusy} onClick={retryHistory}>
              Retry
            </RetryButton>
          </div>
        ) : null}
        {initialTarget === null ? (
          <TurnSkeleton />
        ) : (
          turns.map((turn, index) => (
            <div
              key={turn.id}
              style={
                index === turns.length - 1
                  ? { viewTransitionName: "chat-prompt" }
                  : undefined
              }
            >
              <RequestTurn
                summary={turn.summary}
                dashboardTitle={dashboardTitle}
                loadSteps={index >= loadFrom || turn.id === requestId}
                onRetried={appended}
                buildOnScreen={pane && turn.id === requestId}
                opensRunsIn={pane ? "build" : "chat"}
                latest={index === turns.length - 1}
                about={subject}
                answerActions={
                  !readOnly &&
                  index === turns.length - 1 &&
                  requestOrigin(turn.summary) === "observe" &&
                  turn.summary.requested_by.id === principal.id ? (
                    <AskAgainButton
                      prompt={turn.summary.prompt}
                      subject={subject}
                      chatId={chatId ?? undefined}
                      disabled={active}
                      onSent={openSent}
                      afterSent={focusList}
                    />
                  ) : undefined
                }
              />
            </div>
          ))
        )}
      </div>
    </>
  )

  return (
    <div
      className={cn(
        "flex flex-col items-center",
        pane ? "min-h-0 flex-1" : "min-h-full"
      )}
    >
      {pane ? (
        <ScrollPane className="flex w-full flex-1 flex-col">
          {transcript}
        </ScrollPane>
      ) : (
        transcript
      )}
      {readOnly ? null : (
        <div
          style={{ viewTransitionName: "chat-composer" }}
          className={cn(
            "w-full bg-background pt-2 pb-1",
            pane ? "flex-none" : "sticky -bottom-7 max-w-[760px]"
          )}
        >
          {suggestsFollowUps ? (
            <div className="pb-2">
              <ExampleChips
                questionsOnly
                label="Suggested follow-ups"
                disabled={followUp.pending}
                onChoose={(example) => void followUp.ask(example.prompt)}
              />
            </div>
          ) : null}
          <ConversationComposer
            chatId={chatId}
            subject={subject}
            dashboardTitle={dashboardTitle}
            panel={panel}
            disabled={waitingForAnswer}
            runningRequestId={runningRequestId}
            onSent={openSent}
          />
          {!agentOff ? (
            <p className="mt-1.5 text-center text-xs text-muted-foreground">
              {hasDashboard
                ? "The dashboard changes only when you choose Revise the dashboard on an answer."
                : waitingForAnswer
                  ? "Wait for the answer, or cancel the run, then ask a follow-up."
                  : "The agent sees the earlier messages of this chat."}
            </p>
          ) : null}
        </div>
      )}
    </div>
  )
}

function subjectOf(
  about:
    { dashboard_id: string | null; observation_id: string | null } | undefined
): Subject | null {
  if (about?.dashboard_id != null) {
    return { kind: "dashboard", id: about.dashboard_id }
  }
  if (about?.observation_id != null) {
    return { kind: "observation", id: about.observation_id }
  }
  return null
}

export function Conversation({
  requestId,
  layout,
  asks = true,
  panel = null,
  onSent,
}: {
  requestId: string
  layout: ConversationLayout
  asks?: boolean
  panel?: ChatPanel | null
  onSent?: (request: ObserveRequest) => void
}) {
  const queryClient = useQueryClient()
  const root = useObserveRequest(requestId)
  const [sent, setSent] = useState<ObserveRequest[]>([])
  const chatId = root.data?.chat_id ?? undefined
  const readsChat = chatId !== undefined
  const chat = useChat(chatId)
  const subject = subjectOf(
    readsChat
      ? chat.data?.chat
      : { dashboard_id: root.data?.dashboard_id ?? null, observation_id: null }
  )
  const dashboardId = subject?.kind === "dashboard" ? subject.id : null
  const readOnly =
    root.data?.read_only === true || root.data?.redacted === true || !asks
  const readsDashboard = dashboardId !== null && !readOnly
  const revisions = useQuery({
    ...revisionsQuery(dashboardId ?? ""),
    enabled: readsDashboard && !readsChat,
  })
  const latest = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: readsDashboard,
  })

  const history = readsChat ? chat.data?.turns : revisions.data

  const turns = useMemo<Turn[]>(() => {
    if (root.data === undefined) return []
    const summaries = new Map<string, TurnSummary>()
    for (const request of sent) summaries.set(request.id, request)
    for (const turn of history ?? []) summaries.set(turn.id, turn)
    summaries.set(requestId, root.data)
    const known = [...summaries.values()]
    const ids = orderTurns(requestId, known)
    return ids.flatMap((id) => {
      const summary = summaries.get(id)
      return summary === undefined ? [] : [{ id, summary }]
    })
  }, [root.data, history, sent, requestId])

  if (
    root.error instanceof ApiError &&
    (root.error.status === 404 ||
      (root.data === undefined && root.error.status === 400))
  ) {
    return (
      <RequestNotFound
        removed={root.data !== undefined}
        pane={layout === "pane"}
      />
    )
  }

  if (root.data === undefined) {
    if (root.isError) {
      return (
        <LoadError
          message={root.error.message}
          busy={root.isFetching}
          onRetry={() => void root.refetch()}
        />
      )
    }
    return <ConversationSkeleton />
  }

  if (readsChat && chat.data === undefined) {
    return chat.isError ? (
      <LoadError
        message={chat.error.message}
        busy={chat.isFetching}
        onRetry={() => void chat.refetch()}
      />
    ) : (
      <ConversationSkeleton />
    )
  }

  return (
    <ConversationView
      requestId={requestId}
      layout={layout}
      chatId={root.data.chat_id ?? null}
      subject={subject}
      dashboardTitle={latest.data?.dashboard.title}
      turns={turns}
      readOnly={readOnly}
      ready={
        readsChat ||
        !readsDashboard ||
        revisions.data !== undefined ||
        revisions.isError
      }
      historyFailed={
        !readsChat && revisions.isError && revisions.data === undefined
      }
      retryHistory={() => void revisions.refetch()}
      historyBusy={revisions.isFetching}
      panel={panel}
      onSent={(request) => {
        setSent((previous) => [...previous, request])
        onSent?.(request)
      }}
    />
  )
}
