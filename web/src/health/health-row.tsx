import { useId, useState } from "react"
import { Link } from "@tanstack/react-router"
import {
  ChevronDownIcon,
  ChevronRightIcon,
  LoaderCircleIcon,
} from "lucide-react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell } from "@/components/table-card"
import { Button, buttonVariants } from "@/components/ui/button"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"

import {
  groupProblems,
  kindLabels,
  kindTones,
  problemText,
  repairAction,
  subjectNoun,
  subjectSummary,
  type HealthItem,
  type ProblemGroup,
} from "./health-labels"

type Props = {
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
}

export function HealthRow({ item, agentAvailable, pending, onRepair }: Props) {
  return (
    <tr>
      <Cell className="align-top wrap-anywhere">
        <Link
          {...dashboardLink(item.id, item.title)}
          className="font-medium underline-offset-2 hover:underline"
        >
          {item.title}
        </Link>
        <span className="block text-xs font-normal text-muted-foreground">
          {item.folder.title}
        </span>
      </Cell>
      <Cell className="align-top">
        <ul className="flex flex-col gap-1.5">
          {groupProblems(item.problems).map((group) => (
            <ProblemGroupLine
              key={JSON.stringify([group.kind, group.message])}
              group={group}
            />
          ))}
        </ul>
      </Cell>
      <Cell className="align-top whitespace-nowrap">
        <span className="hidden narrow:inline">Checked </span>
        <RelativeTime iso={item.checked_at} fallback="Not checked yet" />
        {item.skipped === "depends_on_viewer" ? (
          <span className="block text-xs text-muted-foreground narrow:inline narrow:before:content-['_·_']">
            Skipped: reads the viewer
          </span>
        ) : item.checked_version === null ? null : (
          <span className="block text-xs text-muted-foreground narrow:inline narrow:before:content-['_·_']">
            Version {item.checked_version}
          </span>
        )}
      </Cell>
      <Cell className="align-top whitespace-nowrap">
        <span className="hidden narrow:inline">Opened </span>
        <RelativeTime iso={item.last_viewed_at} fallback="Never" />
      </Cell>
      <Cell data-cell="action" className="text-right align-top">
        <RowAction
          item={item}
          agentAvailable={agentAvailable}
          pending={pending}
          onRepair={onRepair}
        />
      </Cell>
    </tr>
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

function RowAction({ item, agentAvailable, pending, onRepair }: Props) {
  const action = repairAction(item, agentAvailable)
  switch (action.kind) {
    case "queued":
      return <StatusLabel tone="running">Repair queued</StatusLabel>
    case "running":
      return <StatusLabel tone="running">Repairing</StatusLabel>
    case "repaired":
      return (
        <span className="inline-flex flex-wrap items-center justify-end gap-x-2 gap-y-1">
          <StatusLabel tone="ok">Repaired</StatusLabel>
          <Link
            to="/d/$dashboardId/versions"
            params={{ dashboardId: item.id }}
            search={
              action.version > 1
                ? { from: action.version - 1, to: action.version }
                : { to: action.version }
            }
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            See changes
          </Link>
        </span>
      )
    case "failed":
      return (
        <span className="flex flex-col items-end gap-1">
          <StatusLabel tone="critical">Repair failed</StatusLabel>
          {action.error === null ? null : (
            <span className="max-w-64 text-left text-xs wrap-anywhere whitespace-normal text-muted-foreground">
              {action.error}
            </span>
          )}
          {action.canRetry ? (
            <RepairButton
              disabledReason={action.disabledReason}
              pending={pending}
              onRepair={onRepair}
            />
          ) : null}
        </span>
      )
    case "repair":
      return (
        <RepairButton
          disabledReason={action.disabledReason}
          pending={pending}
          onRepair={onRepair}
        />
      )
    case "open":
      return (
        <Link
          {...dashboardLink(item.id, item.title)}
          className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
        >
          Open
        </Link>
      )
  }
}

function RepairButton({
  disabledReason,
  pending,
  onRepair,
}: {
  disabledReason: string | null
  pending: boolean
  onRepair: () => void
}) {
  const reasonId = useId()
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={disabledReason !== null || pending}
        focusableWhenDisabled
        aria-busy={pending}
        aria-describedby={disabledReason === null ? undefined : reasonId}
        title={disabledReason ?? undefined}
        onClick={onRepair}
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : null}
        Repair
      </Button>
      {disabledReason === null ? null : (
        <span id={reasonId} className="sr-only">
          {disabledReason}
        </span>
      )}
    </>
  )
}
