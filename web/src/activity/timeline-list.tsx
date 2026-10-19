import {
  Fragment,
  useEffect,
  useLayoutEffect,
  useRef,
  type ReactNode,
  type RefObject,
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

export function useStickToBottom(
  ref: RefObject<HTMLElement | null>,
  depsKey: string
) {
  const nearBottom = useRef(true)

  useEffect(() => {
    const element = ref.current
    if (element === null) return
    const onScroll = () => {
      nearBottom.current =
        element.scrollHeight - element.scrollTop - element.clientHeight <=
        stickThresholdPx
    }
    element.addEventListener("scroll", onScroll, { passive: true })
    return () => element.removeEventListener("scroll", onScroll)
  }, [ref])

  useLayoutEffect(() => {
    const element = ref.current
    if (element === null || !nearBottom.current) return
    element.scrollTop = element.scrollHeight
  }, [ref, depsKey])
}

function Message({ children }: { children: ReactNode }) {
  return (
    <div className="px-4 py-8 text-center text-[13px] text-muted-foreground">
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
      <div className="flex flex-col gap-[18px] px-4 pt-4 pb-2">
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
      </div>
    )
  }
  if (error) {
    return (
      <Message>
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
