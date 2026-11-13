import { StatusLabel } from "@/components/status-label"

import {
  kindLabels,
  kindTones,
  subjectSummary,
  type ProblemGroup,
} from "./health-labels"

export function KindSummary({ group }: { group: ProblemGroup }) {
  const summary = subjectSummary(group)
  return (
    <>
      <StatusLabel tone={kindTones[group.kind]}>
        {kindLabels[group.kind]}
        {summary === null ? null : <span aria-hidden="true">·</span>}
      </StatusLabel>
      {summary === null ? null : (
        <span className="font-normal wrap-anywhere">{summary}</span>
      )}
    </>
  )
}
