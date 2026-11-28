import { sentence } from "@/api/errors"
import { RunLine } from "@/conversation/run-answer"

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
  return <RunLine run={run} agentName={agentName} />
}
