import type { ReactNode } from "react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"

import {
  lastRunProblem,
  summaryLabels,
  type HealthJob,
  type HealthSummary,
} from "./health-labels"

export function SummaryLine({
  summary,
  job,
  action,
}: {
  summary: HealthSummary | null
  job: HealthJob
  action?: ReactNode
}) {
  const runProblem = lastRunProblem(job)
  return (
    <div
      role="status"
      className="mb-3.5 flex flex-wrap items-center gap-x-5 gap-y-2 text-[13px]"
    >
      {(summary === null ? [] : summaryLabels(summary)).map((label) => (
        <span key={label.label} className="font-medium">
          <StatusLabel tone={label.tone}>{label.label}</StatusLabel>
        </span>
      ))}
      <span className="ml-auto flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground">
        {runProblem === null ? null : (
          <StatusLabel tone={runProblem.tone}>{runProblem.label}</StatusLabel>
        )}
        {job.running ? (
          <StatusLabel tone="running">Checking now</StatusLabel>
        ) : job.last_finished_at === null ? (
          <span>No check has finished yet</span>
        ) : (
          <span>
            Last checked <RelativeTime iso={job.last_finished_at} />
          </span>
        )}
        {action}
      </span>
    </div>
  )
}
