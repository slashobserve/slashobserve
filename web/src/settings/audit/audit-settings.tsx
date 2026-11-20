import { useState } from "react"
import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import { serviceAccountsQuery } from "@/service-accounts/queries"
import { loadProblemText } from "@/service-accounts/queries"
import { usersQuery } from "@/people/queries"
import { EmptyState } from "@/components/empty-state"
import { FilterToolbar } from "@/components/filter-toolbar"
import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"
import { formatDateTime } from "@/lib/time"

import {
  auditKinds,
  auditRanges,
  describeAuditEvent,
  type ActivityEvent,
  type ActivityType,
  type AuditRange,
} from "./audit-events"
import { auditInfiniteQuery } from "./queries"

const anyValue = "any"

export function AuditSettings() {
  const [actor, setActor] = useState<string | null>(null)
  const [type, setType] = useState<ActivityType | null>(null)
  const [range, setRange] = useState<AuditRange>("week")
  const query = useInfiniteQuery(auditInfiniteQuery({ actor, type, range }))
  const events = query.data?.pages.flatMap((page) => page.activity) ?? []
  const filtered = actor !== null || type !== null || range !== "week"

  return (
    <SettingsSection
      title="Audit log"
      description="Who changed roles, keys, webhooks, connected agents, outbound access, Grafana connections, public links and comment access, and when. Only Admins can see this."
    >
      <SettingsCard
        title="Admin changes"
        hint="Newest first. A secret is never shown, only that it changed."
      >
        <FilterToolbar
          label="Filter the audit log"
          onClear={
            filtered
              ? () => {
                  setActor(null)
                  setType(null)
                  setRange("week")
                }
              : undefined
          }
        >
          <ActorSelect value={actor} onChange={setActor} />
          <KindSelect value={type} onChange={setType} />
          <RangeSelect value={range} onChange={setRange} />
        </FilterToolbar>
        {query.isError && query.data === undefined ? (
          <SettingsProblem
            title="Could not load the audit log"
            detail={loadProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : query.data === undefined ? (
          <div className="flex flex-col gap-3 border-t p-4">
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-5 w-5/6" />
            <Skeleton className="h-5 w-2/3" />
          </div>
        ) : events.length === 0 ? (
          <EmptyState title="No admin changes match">
            {filtered
              ? "Widen the time range or clear a filter."
              : "Changes to roles, keys, webhooks and connections appear here as they happen."}
          </EmptyState>
        ) : (
          <>
            <ul aria-label="Admin changes" className="divide-y border-t">
              {events.map((event) => (
                <AuditRow key={event.seq} event={event} />
              ))}
            </ul>
            {query.isFetchNextPageError ? (
              <SettingsProblem
                title="Could not load more"
                detail={loadProblemText(query.error)}
                retrying={query.isFetchingNextPage}
                onRetry={() => void query.fetchNextPage()}
              />
            ) : null}
            {query.hasNextPage ? (
              <div className="border-t p-3">
                <Button
                  variant="outline"
                  className="narrow:h-11"
                  disabled={query.isFetchingNextPage}
                  focusableWhenDisabled
                  aria-busy={query.isFetchingNextPage}
                  onClick={() => void query.fetchNextPage()}
                >
                  {query.isFetchingNextPage ? (
                    <LoaderCircleIcon aria-hidden className="animate-spin" />
                  ) : null}
                  Load older changes
                </Button>
              </div>
            ) : null}
          </>
        )}
      </SettingsCard>
    </SettingsSection>
  )
}

function AuditRow({ event }: { event: ActivityEvent }) {
  return (
    <li className="flex flex-wrap items-baseline gap-x-4 gap-y-1 px-4 py-2.5 text-[13px]">
      <time
        dateTime={event.created_at}
        title={formatDateTime(event.created_at)}
        className="w-24 flex-none text-muted-foreground narrow:w-full"
      >
        <RelativeTime iso={event.created_at} />
      </time>
      <span className="min-w-[180px] flex-1 wrap-anywhere">
        <span className="font-medium">{event.actor.label}</span>{" "}
        {describeAuditEvent(event)}
      </span>
    </li>
  )
}

function ActorSelect({
  value,
  onChange,
}: {
  value: string | null
  onChange: (next: string | null) => void
}) {
  const people = useQuery(usersQuery())
  const accounts = useQuery(serviceAccountsQuery())
  const options = [
    { value: anyValue, label: "Anyone" },
    ...(people.data ?? []).map((person) => ({
      value: person.id,
      label: person.name === "" ? person.email : person.name,
    })),
    ...(accounts.data ?? []).map((account) => ({
      value: account.id,
      label: `${account.name} (service account)`,
    })),
  ]
  return (
    <Select
      items={options}
      value={value ?? anyValue}
      onValueChange={(next) => onChange(next === anyValue ? null : next)}
    >
      <SelectTrigger size="sm" aria-label="Who" className="narrow:h-11">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function KindSelect({
  value,
  onChange,
}: {
  value: ActivityType | null
  onChange: (next: ActivityType | null) => void
}) {
  const options = [{ value: anyValue, label: "Any change" }, ...auditKinds]
  return (
    <Select
      items={options}
      value={value ?? anyValue}
      onValueChange={(next) =>
        onChange(next === anyValue ? null : (next as ActivityType))
      }
    >
      <SelectTrigger
        size="sm"
        aria-label="Kind of change"
        className="narrow:h-11"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function RangeSelect({
  value,
  onChange,
}: {
  value: AuditRange
  onChange: (next: AuditRange) => void
}) {
  return (
    <Select
      items={auditRanges}
      value={value}
      onValueChange={(next) => {
        if (next !== null) onChange(next)
      }}
    >
      <SelectTrigger size="sm" aria-label="Time" className="narrow:h-11">
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {auditRanges.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
