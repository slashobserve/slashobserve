import { useEffect, useRef } from "react"

import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { formatDateTime } from "@/lib/time"
import { cn } from "@/lib/utils"

import {
  actorText,
  originText,
  outcomeLabels,
  outcomeTones,
  targetText,
} from "./event-text"
import type { AuditEvent } from "./stream"

export const hiddenBesideDetail = "@min-[1001px]:hidden"

export function AuditTable({
  events,
  selected,
  panelOpen,
  onSelect,
}: {
  events: readonly AuditEvent[]
  selected: string | null
  panelOpen: boolean
  onSelect: (id: string) => void
}) {
  const besideDetail = panelOpen ? hiddenBesideDetail : undefined
  const body = useRef<HTMLTableSectionElement>(null)
  useEffect(() => {
    body.current
      ?.querySelector('[aria-current="true"]')
      ?.scrollIntoView?.({ block: "nearest" })
  }, [selected])
  return (
    <TableCard
      label="Audit events"
      minWidth="var(--audit-table-min)"
      className={cn(
        "[--audit-table-min:720px]",
        panelOpen && "@min-[1001px]:[--audit-table-min:460px]"
      )}
      stackOnNarrow
    >
      <thead>
        <tr>
          <HeaderCell>Action</HeaderCell>
          <HeaderCell>Time</HeaderCell>
          <HeaderCell>Actor</HeaderCell>
          <HeaderCell>Outcome</HeaderCell>
          <HeaderCell className={besideDetail}>Target</HeaderCell>
          <HeaderCell className={besideDetail}>Origin</HeaderCell>
        </tr>
      </thead>
      <tbody ref={body}>
        {events.map((event) => (
          <tr
            key={event.id}
            className={cn(event.id === selected && "bg-selection")}
          >
            <Cell>
              <button
                type="button"
                aria-current={event.id === selected ? "true" : undefined}
                onClick={() => onSelect(event.id)}
                className="rounded-control text-left font-mono text-xs outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
              >
                {event.action}
              </button>
            </Cell>
            <Cell
              data-label="Time"
              className="whitespace-nowrap text-muted-foreground"
            >
              <time
                dateTime={event.occurred_at}
                title={formatDateTime(event.occurred_at)}
              >
                <RelativeTime iso={event.occurred_at} />
              </time>
            </Cell>
            <Cell data-label="Actor" className="wrap-anywhere">
              {actorText(event)}
            </Cell>
            <Cell data-label="Outcome">
              <StatusLabel
                tone={outcomeTones[event.outcome]}
                title={event.reason ?? undefined}
              >
                {outcomeLabels[event.outcome]}
              </StatusLabel>
            </Cell>
            <Cell
              data-label="Target"
              className={cn("wrap-anywhere", besideDetail)}
            >
              {targetText(event)}
            </Cell>
            <Cell data-label="Origin" className={besideDetail}>
              {originText(event)}
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
