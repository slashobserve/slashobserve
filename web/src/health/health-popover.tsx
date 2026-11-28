import type { ReactElement, ReactNode } from "react"

import { StatusLabel } from "@/components/status-label"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import { CheckLine } from "./check-line"
import {
  awaitsRecheck,
  healthyLabel,
  type HealthItem,
  type HealthProblem,
} from "./health-labels"
import { ProblemGroups } from "./problem-groups"
import { RecheckNote } from "./recheck-pending"

type Align = "start" | "end"

export function HealthyPopover({
  item,
  trigger,
  align,
}: {
  item: HealthItem
  trigger: ReactElement
  align: Align
}) {
  return (
    <Popover>
      <PopoverTrigger render={trigger}>
        <StatusLabel tone="ok">{healthyLabel}</StatusLabel>
      </PopoverTrigger>
      <PopoverContent align={align} className="w-80">
        <PopoverTitle>Health</PopoverTitle>
        <p>No problems found</p>
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
      </PopoverContent>
    </Popover>
  )
}

export function HealthPopoverContent({
  item,
  problems,
  align,
  children,
}: {
  item: HealthItem
  problems: readonly HealthProblem[]
  align: Align
  children: ReactNode
}) {
  return (
    <PopoverContent align={align} className="w-80">
      <PopoverTitle>Health</PopoverTitle>
      {awaitsRecheck(item) ? (
        <RecheckNote item={item} />
      ) : (
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
      )}
      <ProblemGroups problems={problems} />
      <div>{children}</div>
    </PopoverContent>
  )
}
