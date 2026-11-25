import { Link } from "@tanstack/react-router"

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

const runLines = {
  queued: "Observe Agent is working on it.",
  running: "Observe Agent is working on it.",
  canceling: "Stopping.",
  canceled: "Stopped.",
  succeeded: "Observe Agent finished.",
  failed: "Observe Agent could not finish.",
}

function runLine(
  request: ReturnType<typeof useObserveRequest>["data"],
  sentStatus: keyof typeof runLines
) {
  if (request?.state === "canceling" || request?.state === "canceled") {
    return runLines[request.state]
  }
  return runLines[request?.status ?? sentStatus]
}

function BuiltinRun({
  id,
  sentStatus,
}: {
  id: string
  sentStatus: keyof typeof runLines
}) {
  const request = useObserveRequest(id)
  return (
    <div className="flex items-center gap-2">
      <p role="status" className="text-xs text-muted-foreground">
        {runLine(request.data, sentStatus)}{" "}
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
