import {
  Fragment,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import { nextDayStart, withDayBreaks } from "./day-label"
import { firstNewVersion, type TimelineEntry } from "./timeline-model"

const stickThresholdPx = 48

type OlderVersions = {
  available: boolean
  fetching: boolean
  error: string | null
  onLoad: () => void
}

type Props = {
  entries: readonly TimelineEntry[]
  loading: boolean
  error: boolean
  onRetry: () => void
  filtered: boolean
  older: OlderVersions
  renderEntry: (entry: TimelineEntry) => ReactNode
}

function scrollToEnd(element: HTMLElement) {
  element.scrollTop = element.scrollHeight
}

export function useStickToBottom(depsKey: string) {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const nearBottom = useRef(true)

  useLayoutEffect(() => {
    if (element === null) return
    nearBottom.current = true
    const onScroll = () => {
      nearBottom.current =
        element.scrollHeight - element.scrollTop - element.clientHeight <=
        stickThresholdPx
    }
    element.addEventListener("scroll", onScroll, { passive: true })
    return () => element.removeEventListener("scroll", onScroll)
  }, [element])

  useLayoutEffect(() => {
    if (element === null || !nearBottom.current) return
    scrollToEnd(element)
  }, [element, depsKey])

  return setElement
}

function useToday(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = setTimeout(
      () => setNow(Date.now()),
      nextDayStart(now) - Date.now()
    )
    return () => clearTimeout(timer)
  }, [now])
  return now
}

function Message({ children, role }: { children: ReactNode; role?: "alert" }) {
  return (
    <div
      role={role}
      className="px-4 py-8 text-center text-[13px] text-muted-foreground"
    >
      {children}
    </div>
  )
}

export function TimelineList({
  entries,
  loading,
  error,
  onRetry,
  filtered,
  older,
  renderEntry,
}: Props) {
  const list = useRef<HTMLOListElement>(null)
  const loadButton = useRef<HTMLButtonElement>(null)
  const knownVersions = useRef<ReadonlySet<number> | null>(null)
  const { fetching } = older
  const today = useToday()

  useEffect(() => {
    const known = knownVersions.current
    if (known === null || fetching) return
    knownVersions.current = null
    const first = firstNewVersion(entries, known)
    if (first === null) return
    const active = document.activeElement
    const onBody = !active || active === document.body
    if (!onBody && active !== loadButton.current) return
    list.current
      ?.querySelector<HTMLElement>(`a[data-version="${first}"]`)
      ?.focus()
  }, [entries, fetching])

  const loadOlder = () => {
    knownVersions.current = new Set(
      entries.flatMap((entry) =>
        entry.kind === "version" ? [entry.version.version] : []
      )
    )
    older.onLoad()
  }

  if (loading) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2 px-4 pt-1 pb-4">
        <span className="sr-only">Loading activity</span>
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
      </div>
    )
  }
  if (error) {
    return (
      <Message role="alert">
        <p>Could not load activity.</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={onRetry}
        >
          Retry
        </Button>
      </Message>
    )
  }
  if (entries.length === 0) {
    return (
      <Message>
        {filtered ? (
          "No activity matches these filters."
        ) : (
          <>
            <p>No activity yet.</p>
            <p className="mt-1 text-xs">
              Comment on a panel from its menu, or write below.
            </p>
          </>
        )}
      </Message>
    )
  }
  return (
    <>
      {older.available ? (
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-2 px-4 pt-2">
          {older.error === null ? null : (
            <p role="alert" className="text-center text-xs text-destructive">
              Could not load older versions: {older.error}
            </p>
          )}
          <Button
            ref={loadButton}
            type="button"
            variant="outline"
            size="sm"
            className="text-foreground"
            disabled={fetching}
            focusableWhenDisabled
            aria-busy={fetching}
            onClick={loadOlder}
          >
            Load older versions
          </Button>
        </div>
      ) : null}
      <ol
        ref={list}
        aria-label="Activity"
        className="flex flex-col px-4 pt-1 pb-4"
      >
        {withDayBreaks(entries, today).map((row) =>
          row.kind === "day" ? (
            <li
              key={row.key}
              className="flex items-center gap-2 pt-3 pb-1 text-xs text-muted-foreground"
            >
              {row.label}
              <span aria-hidden="true" className="h-px flex-1 bg-border" />
            </li>
          ) : (
            <Fragment key={row.entry.key}>{renderEntry(row.entry)}</Fragment>
          )
        )}
      </ol>
    </>
  )
}
