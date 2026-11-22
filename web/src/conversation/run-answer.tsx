import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import type { Message, Thread } from "@/comments/thread-queries"
import { observeRequestQuery } from "@/observe/queries"

import {
  AnswerCard,
  reviseFailureText,
  type RevisionAsked,
} from "./answer-card"
import { reviseBlock, threadPlace, type AgentGate } from "./delivery"
import { planRevise, subjectOf } from "./send-plan"
import { askedAgentName, runLineText, type Run } from "./thread-state"
import { useDeliver } from "./use-composer"

const runLinkClass = "underline underline-offset-2 hover:text-foreground"

export function RunLink({ runId }: { runId: string }) {
  return (
    <Link
      to="/c/$requestId"
      params={{ requestId: runId }}
      className={runLinkClass}
    >
      View the run
    </Link>
  )
}

export function RunLine({ message, run }: { message: Message; run: Run }) {
  const builtin = run.assigned_kind === "builtin"
  const request = useQuery({
    ...observeRequestQuery(run.id),
    enabled: builtin,
    retry: false,
  })
  const status = request.data?.status ?? run.status
  return (
    <p role="status" className="text-xs text-muted-foreground">
      {runLineText(run, status, askedAgentName(message, run))}
      {builtin ? (
        <>
          {" · "}
          <RunLink runId={run.id} />
        </>
      ) : null}
    </p>
  )
}

type MessageAnswer = NonNullable<Message["answer"]>

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
  const { deliver, pending } = useDeliver(subject)
  const findingId = answer.finding.id
  const runId = answer.finding.agent_request_id
  const mayRevise =
    subject?.kind === "dashboard" && thread.allowed_actions.includes("reply")

  async function revise(): Promise<RevisionAsked> {
    const plan = planRevise(
      threadPlace(thread.id, thread),
      subject,
      findingId,
      crypto.randomUUID()
    )
    if ("error" in plan) {
      toast.error(plan.error)
      return false
    }
    try {
      await deliver(plan)
      return true
    } catch (error) {
      if (error instanceof ApiError && error.code === "already_applied") {
        return true
      }
      toast.error(reviseFailureText(error))
      return false
    }
  }

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
