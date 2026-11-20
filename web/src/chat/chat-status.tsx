import { StatusLabel } from "@/components/status-label"
import type { RequestSummary } from "@/observe/queries"
import { cn } from "@/lib/utils"

import { chatOutcome, chatState } from "./chat-state"

export function ChatStatus({
  request,
  truncate = false,
}: {
  request: RequestSummary
  truncate?: boolean
}) {
  const state = chatState(request.state)
  const outcome = chatOutcome(request)
  return (
    <>
      <StatusLabel tone={state.tone}>{state.word}</StatusLabel>
      <span
        title={truncate ? outcome : undefined}
        className={cn(
          truncate && "max-w-[260px] truncate narrow:max-w-[180px]"
        )}
      >
        {outcome}
      </span>
    </>
  )
}
