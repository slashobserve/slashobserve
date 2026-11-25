import { Link } from "@tanstack/react-router"

import { runLineText, runProgress, type Run } from "@/conversation/thread-state"
import { sentence, useObserveRequest } from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"

import type { SentMessage } from "./thread-queries"

type RunStartedProps = {
  run: SentMessage["run"]
  refusal: SentMessage["runRefusal"]
  agentName: string
}

export function RunStarted({ run, refusal, agentName }: RunStartedProps) {
  if (refusal !== null) {
    return (
      <p role="status" className="text-xs text-muted-foreground">
        {sentence(refusal.message)}
      </p>
    )
  }
  if (run === null) return null
  if (run.assigned_kind !== "builtin") {
    return (
      <p role="status" className="text-xs text-muted-foreground">
        {agentName} was asked.
      </p>
    )
  }
  return <BuiltinRun id={run.id} sentStatus={run.status} />
}

function BuiltinRun({
  id,
  sentStatus,
}: {
  id: string
  sentStatus: Run["status"]
}) {
  const request = useObserveRequest(id)
  const progress = runProgress(request.data, sentStatus)
  return (
    <div className="flex items-center gap-2">
      <p role="status" className="text-xs text-muted-foreground">
        {runLineText({ assigned_kind: "builtin" }, progress, "Observe Agent")}.{" "}
        <Link
          to="/c/$requestId"
          params={{ requestId: id }}
          className="underline underline-offset-2 hover:text-foreground"
        >
          View the run
        </Link>
      </p>
      <StopRequestButton requestId={id} variant="text" />
    </div>
  )
}
