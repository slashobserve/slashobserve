import { Link } from "@tanstack/react-router"

import { sentence } from "@/observe/queries"

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
  return (
    <p role="status" className="text-xs text-muted-foreground">
      Observe Agent is working on it.{" "}
      <Link
        to="/c/$requestId"
        params={{ requestId: run.id }}
        className="underline underline-offset-2 hover:text-foreground"
      >
        View the run
      </Link>
    </p>
  )
}
