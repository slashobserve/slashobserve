import { useId, useState } from "react"

import { StatusIcon } from "@/components/status-label"
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

export function StepTimeline({ steps, working, reconnecting, latest }: Props) {
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
      {clipped ? (
        <button
          type="button"
          onClick={() => setShowAll(true)}
          className="self-start rounded-sm text-[13px] text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          Show all steps
        </button>
      ) : null}
    </>
  )
}
