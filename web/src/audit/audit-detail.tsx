import { useEffect, useEffectEvent, useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { FilterIcon } from "lucide-react"

import { loadProblemText } from "@/api/errors"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { formatDateTimeMs } from "@/lib/time"
import { SettingsProblem } from "@/settings/settings-card"
import { SidePanel } from "@/shell/side-panel"

import {
  actorText,
  eventData,
  eventFields,
  outcomeLabels,
  outcomeTones,
  targetPath,
} from "./event-text"
import { auditEventQuery } from "./queries"
import { actorQuery, requestQuery, runQuery, targetQuery } from "./query-text"
import { stepFromKey, type Neighbors } from "./step"
import type { AuditEvent } from "./stream"

export function AuditDetail({
  id,
  neighbors,
  onSelect,
  onQuery,
  onClose,
}: {
  id: string
  neighbors: Neighbors
  onSelect: (id: string) => void
  onQuery: (q: string) => void
  onClose: () => void
}) {
  const detail = useQuery(auditEventQuery(id))
  const event = detail.data?.event

  const titleRef = useRef<HTMLHeadingElement>(null)
  const step = useEffectEvent((key: KeyboardEvent) => {
    const next = stepFromKey(
      key,
      neighbors,
      titleRef.current?.closest("aside") ?? null
    )
    if (next === null) return
    key.preventDefault()
    onSelect(next)
  })
  useEffect(() => {
    document.addEventListener("keydown", step)
    return () => document.removeEventListener("keydown", step)
  }, [])
  return (
    <SidePanel
      title={event?.action ?? "Audit event"}
      description={
        event === undefined
          ? undefined
          : `${actorText(event)}, ${formatDateTimeMs(event.occurred_at)}`
      }
      titleRef={titleRef}
      onClose={onClose}
    >
      {detail.isError ? (
        <SettingsProblem
          title="Could not load this event"
          detail={loadProblemText(detail.error)}
          retrying={detail.isFetching}
          onRetry={() => void detail.refetch()}
        />
      ) : detail.data === undefined ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-5 w-5/6" />
          <Skeleton className="h-5 w-2/3" />
        </div>
      ) : (
        <div className="flex flex-col gap-5">
          <EventFields event={detail.data.event} onQuery={onQuery} />
          <Related
            title="Other events in this request"
            empty="No other event was recorded for this request."
            events={detail.data.same_request.filter((row) => row.id !== id)}
            query={
              detail.data.event.request_id === null
                ? null
                : requestQuery(detail.data.event.request_id)
            }
            onSelect={onSelect}
            onQuery={onQuery}
          />
          {detail.data.event.run_id !== null ? (
            <Related
              title="Everything this run did"
              empty="This run recorded nothing else."
              events={detail.data.same_run.filter((row) => row.id !== id)}
              query={runQuery(detail.data.event.run_id)}
              onSelect={onSelect}
              onQuery={onQuery}
            />
          ) : null}
        </div>
      )}
    </SidePanel>
  )
}

function targetField(event: AuditEvent): string {
  return event.target_name ? "target_name" : "target_id"
}

function FilterButton({
  label,
  query,
  onQuery,
}: {
  label: string
  query: string | null
  onQuery: (q: string) => void
}) {
  if (query === null) return null
  return (
    <Button
      variant="ghost"
      size="icon-xs"
      aria-label={label}
      title={label}
      className="ml-1 align-middle"
      onClick={() => onQuery(query)}
    >
      <FilterIcon aria-hidden="true" />
    </Button>
  )
}

function FieldValue({
  event,
  name,
  value,
  onQuery,
}: {
  event: AuditEvent
  name: string
  value: string
  onQuery: (q: string) => void
}) {
  if (name === "outcome") {
    return (
      <StatusLabel tone={outcomeTones[event.outcome]}>
        {outcomeLabels[event.outcome]}
      </StatusLabel>
    )
  }
  if (name === "actor_name") {
    return (
      <>
        {value}
        <FilterButton
          label={`Filter to actor ${value}`}
          query={actorQuery(event)}
          onQuery={onQuery}
        />
      </>
    )
  }
  if (name === targetField(event)) {
    const path = targetPath(event)
    return (
      <>
        {path === null ? (
          value
        ) : (
          <Link to={path} className="underline underline-offset-4">
            {value}
          </Link>
        )}
        <FilterButton
          label={`Filter to target ${value}`}
          query={targetQuery(event)}
          onQuery={onQuery}
        />
      </>
    )
  }
  return value
}

function EventFields({
  event,
  onQuery,
}: {
  event: AuditEvent
  onQuery: (q: string) => void
}) {
  const data = eventData(event)
  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-[minmax(0,10rem)_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-[13px]">
        {eventFields(event).map((field) => (
          <div key={field.name} className="contents">
            <dt className="text-muted-foreground">{field.label}</dt>
            <dd className="m-0 wrap-anywhere">
              <FieldValue
                event={event}
                name={field.name}
                value={field.value}
                onQuery={onQuery}
              />
            </dd>
          </div>
        ))}
      </dl>
      {data !== null ? (
        <div className="flex flex-col gap-1">
          <span className="text-xs text-muted-foreground">Data</span>
          <pre className="m-0 overflow-x-auto rounded-control border bg-muted/40 p-2 font-mono text-xs">
            {data}
          </pre>
        </div>
      ) : null}
    </div>
  )
}

function Related({
  title,
  empty,
  events,
  query,
  onSelect,
  onQuery,
}: {
  title: string
  empty: string
  events: readonly AuditEvent[]
  query: string | null
  onSelect: (id: string) => void
  onQuery: (q: string) => void
}) {
  return (
    <section aria-label={title} className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-[13px] font-semibold">{title}</h3>
        {query !== null && events.length > 0 ? (
          <Button variant="ghost" size="sm" onClick={() => onQuery(query)}>
            Show in the table
          </Button>
        ) : null}
      </div>
      {events.length === 0 ? (
        <p className="text-xs text-muted-foreground">{empty}</p>
      ) : (
        <ul className="flex flex-col">
          {events.map((event) => (
            <li key={event.id}>
              <button
                type="button"
                onClick={() => onSelect(event.id)}
                className="flex w-full items-baseline gap-2 rounded-control px-1.5 py-1 text-left text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="min-w-0 flex-1 truncate font-mono text-xs">
                  {event.action}
                </span>
                <StatusLabel tone={outcomeTones[event.outcome]}>
                  {outcomeLabels[event.outcome]}
                </StatusLabel>
                <span className="text-xs text-muted-foreground tabular-nums">
                  {formatDateTimeMs(event.occurred_at)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
