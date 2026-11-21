import { useId, useMemo, useState } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { EmptyState } from "@/components/empty-state"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { quickRanges } from "@/lib/time"
import { loadProblemText } from "@/service-accounts/queries"
import { SettingsProblem } from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"

import { AuditDetail } from "./audit-detail"
import { ExportMenu, ExportNotice, useAuditExport } from "./audit-export"
import { AuditFacets } from "./audit-facets"
import { AuditFilterBar } from "./audit-filter-bar"
import { AuditHistogram } from "./audit-histogram"
import { AuditTable } from "./audit-table"
import { auditEventsQuery } from "./queries"
import { AuditQueryError, queryProblemOf } from "./query-problem"
import {
  auditWindow,
  defaultFrom,
  defaultTo,
  followsNow,
  sameSearchLater,
  type AuditView,
} from "./search"
import { type AuditEvent, withLiveEvent } from "./stream"
import { useAuditStream } from "./use-audit-stream"

const auditRanges = [
  ...quickRanges,
  { label: "Last 90 days", from: "now-90d", to: "now" },
  { label: "Last 365 days", from: "now-365d", to: "now" },
]

const restartedText =
  "Live follow restarted: the stream could not resume from the last event it showed, so the list was reloaded up to now."

type Frame = { key: string; anchor: number; live: readonly AuditEvent[] }

function freshFrame(key: string): () => Frame {
  return () => ({ key, anchor: Date.now(), live: [] })
}

function viewKey(view: AuditView): string {
  return JSON.stringify([view.q, view.from, view.to])
}

export function AuditSettings({
  view,
  onView,
}: {
  view: AuditView
  onView: (next: AuditView) => void
}) {
  const key = viewKey(view)
  const [frame, setFrame] = useState<Frame>(freshFrame(key))
  if (frame.key !== key) setFrame(freshFrame(key))
  const anchor = frame.anchor
  const refresh = () => setFrame(freshFrame(key))
  const search = (next: AuditView) => {
    refresh()
    setLiveNotice(null)
    onView(next)
  }

  const resolved = useMemo(
    () => auditWindow(view.from, view.to, anchor),
    [view.from, view.to, anchor]
  )
  const window: AuditWindowKey | null =
    resolved.problem === undefined
      ? { q: view.q, since: resolved.since, until: resolved.until }
      : null

  const [draft, setDraft] = useState({ applied: view.q, text: view.q })
  if (draft.applied !== view.q) setDraft({ applied: view.q, text: view.q })

  const [liveOn, setLiveOn] = useState(false)
  const [liveNotice, setLiveNotice] = useState<string | null>(null)
  const liveRows = frame.key === key ? frame.live : []

  const events = useInfiniteQuery({
    ...auditEventsQuery(window ?? { q: "", since: "", until: "" }),
    enabled: window !== null,
    placeholderData: (previous, previousQuery) => {
      const before = previousQuery?.queryKey[2]
      return window !== null &&
        before !== undefined &&
        sameSearchLater(before, window)
        ? previous
        : undefined
    },
  })
  const loaded = useMemo(
    () => events.data?.pages.flatMap((page) => page.events) ?? [],
    [events.data]
  )
  const loadedIds = useMemo(
    () => new Set(loaded.map((event) => event.id)),
    [loaded]
  )
  const shown = [
    ...liveRows.filter((event) => !loadedIds.has(event.id)),
    ...loaded,
  ]

  const canFollow = followsNow(view) && window !== null
  const following = liveOn && canFollow && !events.isError
  const stream = useAuditStream({
    q: view.q,
    enabled: following,
    onEvent: (event) =>
      setFrame((current) => ({
        ...current,
        live: withLiveEvent(current.live, event, loadedIds),
      })),
    onRestart: () => {
      refresh()
      setLiveNotice(restartedText)
    },
    onStop: (message) => {
      setLiveOn(false)
      setLiveNotice(message)
    },
  })

  const exporter = useAuditExport(window)
  const liveId = useId()
  const problem = queryProblemOf(events.error, view.q)
  const filtered =
    view.q !== "" || view.from !== defaultFrom || view.to !== defaultTo

  function run(q: string) {
    search({ ...view, q, event: null })
  }

  return (
    <SettingsSection
      title="Audit log"
      description="Every sign-in, read and change on this server: who did it, through which tool, and how it went. Only Admins can see this."
      action={
        <div className="flex flex-wrap items-center gap-2">
          <TimeRangeControl
            value={{ from: view.from, to: view.to }}
            onChange={(range) => search({ ...view, ...range, event: null })}
            quickRanges={auditRanges}
          />
          <ExportMenu
            disabled={window === null}
            pending={exporter.pending}
            onExport={(format) => void exporter.start(format)}
          />
        </div>
      }
    >
      <AuditFilterBar
        draft={draft.text}
        onDraft={(text) => setDraft({ applied: view.q, text })}
        onRun={() => run(draft.text)}
        problem={problem}
      />
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <div className="flex items-center gap-2">
          <Switch
            id={liveId}
            checked={liveOn && canFollow}
            disabled={!canFollow}
            aria-describedby={`${liveId}-hint`}
            onCheckedChange={(next) => {
              if (next) refresh()
              setLiveOn(next)
              setLiveNotice(null)
            }}
          />
          <Label htmlFor={liveId}>Live</Label>
        </div>
        <p
          id={`${liveId}-hint`}
          role="status"
          className="text-xs text-muted-foreground"
        >
          {!canFollow
            ? "Live follows a time range that ends now."
            : following
              ? stream.connected
                ? "New events appear at the top as they are recorded."
                : "Connecting..."
              : "Turn on to add new events at the top as they are recorded."}
        </p>
      </div>
      {liveNotice !== null ? (
        <Alert role="alert">
          <AlertDescription>{liveNotice}</AlertDescription>
        </Alert>
      ) : null}
      {exporter.notice !== null ? (
        <ExportNotice notice={exporter.notice} onDismiss={exporter.dismiss} />
      ) : null}
      {resolved.problem !== undefined ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{resolved.problem}</AlertDescription>
        </Alert>
      ) : null}
      {window !== null && problem === null ? (
        <>
          <AuditHistogram window={window} />
          <AuditFacets window={window} onQuery={run} />
        </>
      ) : null}
      {window === null || problem !== null ? null : events.isError &&
        events.data === undefined ? (
        <SettingsProblem
          title="Could not load the audit log"
          detail={
            events.error instanceof AuditQueryError
              ? events.error.message
              : loadProblemText(events.error)
          }
          retrying={events.isFetching}
          onRetry={() => void events.refetch()}
        />
      ) : events.data === undefined ? (
        <div className="flex flex-col gap-3 rounded-card border p-4">
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-5 w-5/6" />
          <Skeleton className="h-5 w-2/3" />
        </div>
      ) : shown.length === 0 ? (
        <div className="rounded-card border">
          {filtered ? (
            <EmptyState
              title="No audit events match these filters."
              action={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    search({
                      q: "",
                      from: defaultFrom,
                      to: defaultTo,
                      event: null,
                    })
                  }
                >
                  Clear filters
                </Button>
              }
            />
          ) : (
            <EmptyState title="No audit events in the last 24 hours.">
              Sign-ins, reads and changes appear here as they happen.
            </EmptyState>
          )}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <AuditTable
            events={shown}
            selected={view.event}
            onSelect={(id) => onView({ ...view, event: id })}
          />
          {events.isFetchNextPageError ? (
            <SettingsProblem
              title="Could not load more"
              detail={loadProblemText(events.error)}
              retrying={events.isFetchingNextPage}
              onRetry={() => void events.fetchNextPage()}
            />
          ) : null}
          {events.hasNextPage ? (
            <div>
              <Button
                variant="outline"
                className="narrow:h-11"
                disabled={events.isFetchingNextPage}
                focusableWhenDisabled
                aria-busy={events.isFetchingNextPage}
                onClick={() => void events.fetchNextPage()}
              >
                {events.isFetchingNextPage ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="motion-safe:animate-spin"
                  />
                ) : null}
                Load older events
              </Button>
            </div>
          ) : null}
        </div>
      )}
      {view.event !== null ? (
        <AuditDetail
          id={view.event}
          onSelect={(id) => onView({ ...view, event: id })}
          onQuery={(q) => search({ ...view, q })}
          onClose={() => onView({ ...view, event: null })}
        />
      ) : null}
    </SettingsSection>
  )
}
