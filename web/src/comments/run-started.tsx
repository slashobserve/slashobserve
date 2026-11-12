import { Link } from "@tanstack/react-router"

import { sentence, useObserveRequest } from "@/observe/queries"

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
  succeeded: "Observe Agent finished.",
  failed: "Observe Agent could not finish.",
}

function BuiltinRun({
  id,
  sentStatus,
}: {
  id: string
  sentStatus: keyof typeof runLines
}) {
  const request = useObserveRequest(id)
  const status = request.data?.status ?? sentStatus
  return (
    <p role="status" className="text-xs text-muted-foreground">
      {runLines[status]}{" "}
      <Link
        to="/c/$requestId"
        params={{ requestId: id }}
        className="underline underline-offset-2 hover:text-foreground"
      >
        View the run
      </Link>
    </p>
  )
}
