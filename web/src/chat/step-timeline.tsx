import { Link } from "@tanstack/react-router"
import { useId, useState } from "react"

import { CopyButton } from "@/components/copy-button"
import { StatusIcon } from "@/components/status-label"
import { shortRequestId } from "@/observe/conversation"
import {
  timelinePhases,
  type Step,
  type TimelinePhase,
  type ToolCount,
} from "@/observe/request-model"

const phaseLabels: Record<Step["phase"], string> = {
  discover: "Discovering",
  lint: "Checking queries",
  query: "Running queries",
  save: "Saving",
  repair: "Repairing",
}

const noneExpanded: ReadonlySet<string> = new Set()

type Props = {
  steps: Step[]
  working: boolean
  reconnecting: boolean
  latest?: number
  liveText?: string | null
  requestId?: string
  pane?: boolean
}

function toolText(tools: readonly ToolCount[]): string {
  return tools
    .map(({ tool, count }) => (count > 1 ? `${tool} ${count}` : tool))
    .join(", ")
}

function StepMessages({ id, steps }: { id?: string; steps: Step[] }) {
  return (
    <ul id={id} className="flex flex-col gap-0.5 text-muted-foreground">
      {steps.map((step, index) => (
        <li key={index} className="min-w-0 wrap-anywhere">
          {step.message}
        </li>
      ))}
    </ul>
  )
}

function RequestIdLine({ id, pane }: { id: string; pane: boolean }) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
      <span className="font-mono">Request {shortRequestId(id)}</span>
      <CopyButton
        variant="ghost"
        size="icon-xs"
        text={id}
        label="Copy request ID"
      />
      {pane ? (
        <Link
          to="/c/$requestId"
          params={{ requestId: id }}
          target="_blank"
          rel="noopener"
          className="rounded-control outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          Open in a new tab
        </Link>
      ) : null}
    </div>
  )
}

function PhaseItem({
  run,
  expanded,
  onToggle,
}: {
  run: TimelinePhase
  expanded: boolean
  onToggle: () => void
}) {
  const listId = useId()
  const count = run.steps.length
  const single = count === 1
  const alwaysOpen = run.phase === "save" && !single
  return (
    <li className="flex items-start gap-2 text-[13px]">
      <span className="flex h-[1.45em] items-center">
        <StatusIcon tone={run.errors > 0 ? "warning" : "ok"} />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 flex-wrap gap-x-2">
          <span>
            {phaseLabels[run.phase]}
            {run.errors > 0 ? (
              <span className="sr-only"> needed a fix</span>
            ) : null}
          </span>
          <span className="min-w-0 wrap-anywhere text-muted-foreground">
            {toolText(run.tools)}
          </span>
          {single ? (
            <span className="min-w-0 wrap-anywhere text-muted-foreground">
              {run.steps[0].message}
            </span>
          ) : null}
          {!single && !alwaysOpen ? (
            <button
              type="button"
              aria-expanded={expanded}
              aria-controls={listId}
              onClick={onToggle}
              className="rounded-sm text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              {expanded ? `Hide ${count} steps` : `Show ${count} steps`}
            </button>
          ) : null}
        </span>
        {alwaysOpen || (!single && expanded) ? (
          <StepMessages id={listId} steps={run.steps} />
        ) : null}
      </span>
    </li>
  )
}

export function StepTimeline({
  steps,
  working,
  reconnecting,
  latest,
  liveText,
  requestId,
  pane = false,
}: Props) {
  const [expanded, setExpanded] = useState(noneExpanded)
  const [showAll, setShowAll] = useState(false)
  const runs = timelinePhases(steps)
  const clipped = latest !== undefined && !showAll && runs.length > latest
  const shown = clipped ? runs.slice(runs.length - latest) : runs

  function toggle(key: string) {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <>
      <ol
        aria-label="Progress"
        aria-live="polite"
        aria-relevant="additions"
        className="flex flex-col gap-1.5 empty:hidden"
      >
        {shown.map((run) => (
          <PhaseItem
            key={run.key}
            run={run}
            expanded={expanded.has(run.key)}
            onToggle={() => toggle(run.key)}
          />
        ))}
        {working ? (
          <li className="flex items-center gap-2 text-[13px]">
            <StatusIcon tone="running" />
            <span>Working</span>
          </li>
        ) : null}
        {working && reconnecting ? (
          <li className="pl-[21px] text-[13px] text-muted-foreground">
            Reconnecting...
          </li>
        ) : null}
      </ol>
      {working && liveText ? (
        <div className="flex max-h-[9em] flex-col justify-end overflow-hidden pl-[21px] text-[13px] leading-[1.5]">
          <p
            aria-live="off"
            className="text-[13px] wrap-anywhere whitespace-pre-wrap text-muted-foreground"
          >
            {liveText}
          </p>
        </div>
      ) : null}
      {clipped ? (
        <button
          type="button"
          onClick={() => setShowAll(true)}
          className="self-start rounded-sm text-[13px] text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          Show all steps
        </button>
      ) : null}
      {requestId === undefined ? null : (
        <RequestIdLine id={requestId} pane={pane} />
      )}
    </>
  )
}
