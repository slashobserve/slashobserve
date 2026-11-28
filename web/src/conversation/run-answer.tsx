import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import type { Message, Thread } from "@/comments/thread-queries"
import { observeRequestQuery } from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"
import { cn } from "@/lib/utils"

import { AnswerCard } from "./answer-card"
import { reviseBlock, threadPlace, type AgentGate } from "./delivery"
import { subjectOf } from "./send-plan"
import { runLineText, runProgress, type Run } from "./thread-state"
import { useReviseAnswer } from "./use-revise-answer"

const runLinkClass = "underline underline-offset-2 hover:text-foreground"

export function RunLink({
  runId,
  className,
}: {
  runId: string
  className?: string
}) {
  return (
    <Link
      to="/c/$requestId"
      params={{ requestId: runId }}
      className={cn(runLinkClass, className)}
    >
      View the run
    </Link>
  )
}

export function RunLine({
  run,
  agentName,
}: {
  run: Pick<Run, "id" | "status" | "assigned_kind">
  agentName: string
}) {
  const builtin = run.assigned_kind === "builtin"
  const request = useQuery({
    ...observeRequestQuery(run.id),
    enabled: builtin,
    retry: false,
  })
  const progress = runProgress(request.data, run.status)
  return (
    <div className="flex items-center gap-2">
      <p role="status" className="text-xs text-muted-foreground">
        {runLineText(run, progress, agentName)}
        {builtin ? (
          <>
            {" · "}
            <RunLink runId={run.id} />
          </>
        ) : null}
      </p>
      {builtin ? <StopRequestButton requestId={run.id} variant="text" /> : null}
    </div>
  )
}

type MessageAnswer = NonNullable<Message["answer"]>

const answeredReply = "Answered."

export function answerCoversBody(
  message: Message,
  answer: MessageAnswer | null
): boolean {
  return answer !== null && message.body === answeredReply
}

export function shownAnswer(
  message: Message,
  thread: Thread | undefined
): MessageAnswer | null {
  const answer = message.answer ?? null
  if (answer === null || thread === undefined) return null
  return answer.finding.agent_request_id === null ? null : answer
}

export function RunAnswer({
  answer,
  target,
  thread,
  gate,
}: {
  answer: MessageAnswer
  target: string
  thread: Thread
  gate: AgentGate
}) {
  const subject = subjectOf(target)
  const findingId = answer.finding.id
  const runId = answer.finding.agent_request_id
  const mayRevise =
    subject?.kind === "dashboard" && thread.allowed_actions.includes("reply")

  const { revise, pending } = useReviseAnswer({
    subject,
    place: threadPlace(thread.id, thread),
    revised: subject,
    findingId,
  })

  if (runId === null) return null

  return (
    <AnswerCard
      finding={answer.finding}
      requestId={runId}
      canSave={answer.can_save && subject?.kind !== "observation"}
      revise={
        mayRevise
          ? {
              onRevise: revise,
              pending,
              blocked: reviseBlock(gate, thread.agent?.kind === "builtin"),
              dashboardId: subject.id,
            }
          : undefined
      }
    />
  )
}
