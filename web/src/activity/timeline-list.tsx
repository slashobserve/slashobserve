import {
  Fragment,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import type { TimelineEntry } from "./timeline-model"

const stickThresholdPx = 48

type Props = {
  entries: readonly TimelineEntry[]
  loading: boolean
  error: boolean
  onRetry: () => void
  filtered: boolean
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
  renderEntry,
}: Props) {
  if (loading) {
    return (
      <div aria-busy="true" className="flex flex-col gap-[18px] px-4 pt-4 pb-2">
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
        {filtered
          ? "No activity matches these filters."
          : "Nothing here yet. Comments, agent requests and versions show up here."}
      </Message>
    )
  }
  return (
    <ol
      aria-label="Activity"
      className="flex flex-col gap-[18px] px-4 pt-4 pb-2"
    >
      {entries.map((entry) => (
        <Fragment key={entry.key}>{renderEntry(entry)}</Fragment>
      ))}
    </ol>
  )
}
