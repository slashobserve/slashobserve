import { useId, useMemo, useRef, useState } from "react"
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon, TextSearchIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { quickRanges } from "@/lib/time"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"
import { SettingsProblem } from "@/settings/settings-card"

import { AuditDetail } from "./audit-detail"
import { ExportMenu, ExportNotice, useAuditExport } from "./audit-export"
import { AuditFacets } from "./audit-facets"
import { AuditFilterBar } from "./audit-filter-bar"
import { AuditHistogram } from "./audit-histogram"
import { AuditTable } from "./audit-table"
import { auditInLogsLink } from "./open-in-logs"
import { auditEventsQuery } from "./queries"
import {
  appliedKey,
  appliedOf,
  frameOf,
  needsCheck,
  syncFrame,
  type Frame,
} from "./frame"
import { AuditQueryError, queryProblemOf, rejectionOf } from "./query-problem"
import {
  auditWindow,
  defaultFrom,
  defaultTo,
  followsNow,
  sameSearchLater,
  type AuditView,
} from "./search"
import { neighborsOf } from "./step"
import { withLiveEvent } from "./stream"
import { useAuditStream } from "./use-audit-stream"

const auditRanges = [
  ...quickRanges,
  { label: "Last 90 days", from: "now-90d", to: "now" },
  { label: "Last 365 days", from: "now-365d", to: "now" },
]

const restartedText =
  "Live follow restarted: the stream could not resume from the last event it showed, so the list was reloaded up to now."

type Rejection = { q: string; key: string; error: AuditQueryError }

function stamp(): number {
  return Date.now()
}

const emptyWindow: AuditWindowKey = { q: "", since: "", until: "" }

export function AuditLog({
  view,
  onView,
}: {
  view: AuditView
  onView: (next: AuditView) => void
}) {
  const urlKey = appliedKey(appliedOf(view))
  const [frame, setFrame] = useState<Frame>(() =>
    frameOf(appliedOf(view), urlKey, stamp())
  )
  if (frame.seen !== urlKey) {
    setFrame((current) => syncFrame(current, view, stamp()))
  }
  const { applied, anchor } = frame
  const shownView: AuditView = { ...applied, event: view.event }
  const queryClient = useQueryClient()
  const [rejection, setRejection] = useState<Rejection | null>(null)
  const [liveOn, setLiveOn] = useState(false)
  const [liveNotice, setLiveNotice] = useState<string | null>(null)

  const refresh = () =>
    setFrame((current) => frameOf(current.applied, current.seen, stamp()))

  function apply(next: AuditView, at: number) {
    setLiveNotice(null)
    setFrame((current) => frameOf(appliedOf(next), current.seen, at))
    onView(next)
  }

  const resolved = useMemo(
    () => auditWindow(applied.from, applied.to, anchor),
    [applied.from, applied.to, anchor]
  )
  const window: AuditWindowKey | null =
    resolved.problem === undefined
      ? { q: applied.q, since: resolved.since, until: resolved.until }
      : null

  const latestCheck = useRef(0)

  function search(next: AuditView) {
    latestCheck.current += 1
    setRejection(null)
    if (needsCheck(applied, next) && resolved.problem === undefined) {
      void check(next, latestCheck.current)
      setDraft({ applied: applied.q, text: next.q })
      return
    }
    apply(next, stamp())
  }

  async function check(next: AuditView, ticket: number) {
    const anchor = stamp()
    const range = auditWindow(next.from, next.to, anchor)
    if (range.problem !== undefined) return apply(next, anchor)
    const outcome = await queryClient
      .fetchInfiniteQuery(
        auditEventsQuery({ q: next.q, since: range.since, until: range.until })
      )
      .then(() => null)
      .catch((error: unknown) => error)
    if (ticket !== latestCheck.current) return
    if (outcome instanceof AuditQueryError) {
      setRejection({
        q: next.q,
        key: appliedKey(applied),
        error: outcome,
      })
      return
    }
    apply(next, anchor)
  }

  const [draft, setDraft] = useState({ applied: applied.q, text: applied.q })
  if (draft.applied !== applied.q) {
    setDraft({ applied: applied.q, text: applied.q })
  }

  const liveRows = frame.live

  const events = useInfiniteQuery({
    ...auditEventsQuery(window ?? emptyWindow),
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
  const freshLive = useMemo(
    () => liveRows.filter((event) => !loadedIds.has(event.id)),
    [liveRows, loadedIds]
  )
  const shown = useMemo(() => [...freshLive, ...loaded], [freshLive, loaded])

  const neighbors = useMemo(
    () =>
      view.event === null
        ? neighborsOf([], "")
        : neighborsOf(
            shown.map((event) => event.id),
            view.event
          ),
    [shown, view.event]
  )

  const canFollow = followsNow(shownView) && window !== null
  const following = liveOn && canFollow && !events.isError
  const stream = useAuditStream({
    q: applied.q,
    enabled: following,
    onEvent: (event) => {
      const now = stamp()
      setFrame((current) => ({
        ...current,
        ...withLiveEvent(current, event, loadedIds, now),
      }))
    },
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
  const problem = queryProblemOf(events.error, applied.q)
  const filtered =
    applied.q !== "" || applied.from !== defaultFrom || applied.to !== defaultTo
  const activeRejection =
    rejection !== null &&
    rejection.key === appliedKey(applied) &&
    rejection.q === draft.text
      ? rejection
      : null
  const barProblem =
    activeRejection === null
      ? problem
      : rejectionOf(activeRejection.error, activeRejection.q)

  function run(raw: string) {
    const q = raw.trimEnd()
    setDraft({ applied: applied.q, text: q })
    search({ ...shownView, q, event: null })
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Audit log"
        description="Every sign-in, read and change on this server: who did it, through which tool, and how it went. Only Admins can see this."
        actions={
          <>
            <TimeRangeControl
              value={{ from: applied.from, to: applied.to }}
              onChange={(range) =>
                search({ ...shownView, ...range, event: null })
              }
              quickRanges={auditRanges}
            />
            <Link
              {...auditInLogsLink(applied)}
              className={cn(buttonVariants({ variant: "outline" }))}
            >
              <TextSearchIcon aria-hidden="true" />
              Open in Logs
            </Link>
            <ExportMenu
              disabled={window === null || problem !== null}
              pending={exporter.pending}
              onExport={(format) => void exporter.start(format)}
            />
          </>
        }
      />
      <AuditFilterBar
        draft={draft.text}
        onDraft={(text) => setDraft({ applied: applied.q, text })}
        onRun={() => run(draft.text)}
        problem={barProblem}
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
      <div
        className={cn(
          "flex flex-col gap-4",
          activeRejection !== null && "opacity-60"
        )}
      >
        {window !== null && problem === null ? (
          <>
            <AuditHistogram
              window={window}
              liveCount={freshLive.length}
              onRange={(range) =>
                search({ ...shownView, ...range, event: null })
              }
            />
            <AuditFacets
              window={window}
              query={draft.text}
              live={freshLive}
              onQuery={run}
            />
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
            {view.event === null ? null : (
              <p className="hidden text-xs text-muted-foreground @min-[1001px]:block">
                Target and Origin are shown in the detail panel while it is
                open.
              </p>
            )}
            <AuditTable
              events={shown}
              selected={view.event}
              panelOpen={view.event !== null}
              onSelect={(id) => onView({ ...shownView, event: id })}
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
      </div>
      {view.event !== null ? (
        <AuditDetail
          id={view.event}
          neighbors={neighbors}
          onSelect={(id) => onView({ ...shownView, event: id })}
          onQuery={(q) => search({ ...shownView, q })}
          onClose={() => onView({ ...shownView, event: null })}
        />
      ) : null}
    </div>
  )
}
