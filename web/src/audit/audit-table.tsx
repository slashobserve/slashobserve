import { useMemo, useRef } from "react"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { formatDateTime } from "@/lib/time"

import {
  actorText,
  originText,
  outcomeLabels,
  outcomeTones,
  targetText,
} from "./event-text"
import type { AuditEvent } from "./stream"
import { useBesideDetail } from "./use-beside-detail"

function wrapped(text: string) {
  return text === "" ? null : <span className="py-2 wrap-anywhere">{text}</span>
}

export function auditColumns(
  selected: string | null,
  beside: boolean
): DataGridColumn<AuditEvent>[] {
  const columns: DataGridColumn<AuditEvent>[] = [
    {
      id: "action",
      header: "Action",
      minWidth: 160,
      flex: 1.4,
      cell: (event) => (
        <button
          type="button"
          aria-current={event.id === selected ? "true" : undefined}
          data-event-id={event.id}
          className="min-w-0 truncate rounded-control text-left font-mono text-xs outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {event.action}
        </button>
      ),
    },
    {
      id: "time",
      header: "Time",
      minWidth: 110,
      flex: 0.8,
      cell: (event) => (
        <time
          dateTime={event.occurred_at}
          title={formatDateTime(event.occurred_at)}
          className="whitespace-nowrap text-muted-foreground"
        >
          <RelativeTime iso={event.occurred_at} />
        </time>
      ),
    },
    {
      id: "actor",
      header: "Actor",
      minWidth: 110,
      flex: 1,
      autoHeight: true,
      cell: (event) => wrapped(actorText(event)),
    },
    {
      id: "outcome",
      header: "Outcome",
      minWidth: 100,
      flex: 0.7,
      cell: (event) => (
        <StatusLabel
          tone={outcomeTones[event.outcome]}
          title={event.reason ?? undefined}
        >
          {outcomeLabels[event.outcome]}
        </StatusLabel>
      ),
    },
  ]
  if (beside) return columns
  columns.push(
    {
      id: "target",
      header: "Target",
      minWidth: 130,
      flex: 1.2,
      autoHeight: true,
      cell: (event) => wrapped(targetText(event)),
    },
    {
      id: "origin",
      header: "Origin",
      minWidth: 100,
      flex: 0.8,
      cell: (event) => originText(event),
    }
  )
  return columns
}

const eventId = (event: AuditEvent) => event.id

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
  const container = useRef<HTMLDivElement>(null)
  const detailBeside = useBesideDetail(container)
  const columns = useMemo(
    () => auditColumns(selected, panelOpen && detailBeside),
    [selected, panelOpen, detailBeside]
  )
  return (
    <div
      ref={container}
      onClick={(click) => {
        const id = (click.target as Element)
          .closest("[data-event-id]")
          ?.getAttribute("data-event-id")
        if (id != null) onSelect(id)
      }}
    >
      <DataGrid
        label="Audit events"
        rows={events}
        columns={columns}
        rowId={eventId}
        highlightedRowId={selected}
        revealRowId={selected}
        keepScrollOnPrepend
      />
    </div>
  )
}
