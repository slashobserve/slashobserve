import { ApiError } from "@/api/errors"
import { AuthorMark } from "@/components/author-mark"
import { Button } from "@/components/ui/button"
import { StepTimeline } from "@/chat/step-timeline"
import { showsWorkingRow } from "@/chat/turn-model"
import { TurnFailure } from "@/chat/turn-failure"
import type { AuthorMarkModel } from "@/lib/author"
import type { Block } from "@/observations/queries"
import {
  isTerminal,
  useObserveRequest,
  type ObserveRequest,
} from "@/observe/queries"
import { CancelRequestButton } from "@/observe/request-actions"

import { runBlockBody } from "./block-data"

const observeAgentMark: AuthorMarkModel = {
  kind: "observe-agent",
  text: "/",
  tint: null,
}

const unavailableText = "This run is no longer available."

type Props = { block: Block; onCite: (blockId: string) => void }

function citeLabel(index: number): string {
  return index === 0 ? "Conclusion" : `Panel ${index}`
}

function runHeading(state: ObserveRequest["state"]): string {
  switch (state) {
    case "succeeded":
      return "Observe Agent finished"
    case "canceled":
      return "Observe Agent stopped"
    case "failed":
      return "Observe Agent could not finish"
    default:
      return "Observe Agent is working"
  }
}

function Unavailable() {
  return (
    <p className="rounded-card border px-3.5 py-3 text-[13px] text-muted-foreground">
      {unavailableText}
    </p>
  )
}

function Cites({
  cites,
  onCite,
}: {
  cites: string[]
  onCite: (blockId: string) => void
}) {
  if (cites.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-[12px] text-muted-foreground">Cites</span>
      {cites.map((id, index) => (
        <Button
          key={id}
          variant="ghost"
          size="sm"
          className="h-7"
          onClick={() => onCite(id)}
        >
          {citeLabel(index)}
        </Button>
      ))}
    </div>
  )
}

function RunProgress({
  runId,
  cites,
  onCite,
}: {
  runId: string
  cites: string[]
  onCite: (blockId: string) => void
}) {
  const query = useObserveRequest(runId)
  const request = query.data
  if (request === undefined) {
    if (query.error instanceof ApiError && query.error.status === 404) {
      return <Unavailable />
    }
    return (
      <p className="rounded-card border px-3.5 py-3 text-[13px] text-muted-foreground">
        {query.isError ? "This run could not be loaded." : "Loading the run."}
      </p>
    )
  }
  const finished = isTerminal(request)
  const timeline = (working: boolean) => (
    <StepTimeline
      steps={request.steps}
      working={working}
      reconnecting={query.isError}
      requestId={runId}
    />
  )

  return (
    <section
      aria-label="Observe Agent run"
      aria-busy={!finished}
      className="flex flex-col gap-3 rounded-card border px-3.5 py-3"
    >
      <div className="flex items-center gap-2.5">
        <AuthorMark mark={observeAgentMark} size={24} />
        <h3 className="min-w-0 flex-1 text-[13px] font-semibold">
          {runHeading(request.state)}
        </h3>
        <CancelRequestButton request={request} />
      </div>
      {finished ? (
        <>
          {request.state === "failed" ? (
            <TurnFailure request={request} onRetried={ignoreRetried} />
          ) : null}
          <details>
            <summary className="cursor-pointer text-[13px] text-muted-foreground">
              Show steps
            </summary>
            <div className="flex flex-col gap-2.5 pt-2.5">
              {timeline(false)}
            </div>
          </details>
          <Cites cites={cites} onCite={onCite} />
        </>
      ) : (
        <div className="flex flex-col gap-2.5">
          {timeline(showsWorkingRow(request, query.isError))}
        </div>
      )}
    </section>
  )
}

function ignoreRetried() {}

export function RunBlock({ block, onCite }: Props) {
  const body = runBlockBody(block)
  if (body === null || block.run_id === null) return <Unavailable />
  return <RunProgress runId={body.runId} cites={body.cites} onCite={onCite} />
}
