import { useId, useState } from "react"
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"

import {
  groupProblems,
  kindLabels,
  kindTones,
  problemText,
  subjectNoun,
  subjectSummary,
  type HealthProblem,
  type ProblemGroup,
} from "./health-labels"

export function ProblemGroups({
  problems,
}: {
  problems: readonly HealthProblem[]
}) {
  return (
    <ul className="flex flex-col gap-1.5">
      {groupProblems(problems).map((group) => (
        <ProblemGroupLine
          key={JSON.stringify([group.kind, group.message])}
          group={group}
        />
      ))}
    </ul>
  )
}

function ProblemGroupLine({ group }: { group: ProblemGroup }) {
  const [open, setOpen] = useState(false)
  const listId = useId()
  const summary = subjectSummary(group)
  const text = problemText(group)
  const noun = subjectNoun(group)
  return (
    <li className="flex flex-col items-start gap-0.5">
      <span className="flex flex-wrap items-center gap-x-1.5 font-medium">
        <StatusLabel tone={kindTones[group.kind]}>
          {kindLabels[group.kind]}
        </StatusLabel>
        {summary === null ? null : (
          <span className="font-normal wrap-anywhere">· {summary}</span>
        )}
      </span>
      {text === null ? null : (
        <span className="text-xs wrap-anywhere text-muted-foreground narrow:hidden">
          {text}
        </span>
      )}
      {group.subjects.length > 1 ? (
        <div className="flex flex-col items-start narrow:hidden">
          <Button
            variant="ghost"
            size="xs"
            className="-ml-2 font-normal text-muted-foreground"
            aria-expanded={open}
            aria-controls={listId}
            onClick={() => setOpen((was) => !was)}
          >
            {open ? (
              <ChevronDownIcon aria-hidden="true" />
            ) : (
              <ChevronRightIcon aria-hidden="true" />
            )}
            {open ? `Hide ${noun}` : `Show ${noun}`}
          </Button>
          <ul
            id={listId}
            hidden={!open}
            className="flex list-disc flex-col gap-0.5 pl-4 text-xs text-muted-foreground"
          >
            {group.subjects.map((subject, index) => (
              <li key={index} className="wrap-anywhere">
                {subject}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </li>
  )
}
