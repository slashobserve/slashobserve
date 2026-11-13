import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { useRouteContext } from "@tanstack/react-router"

import { withDayBreaks } from "@/activity/day-label"
import { actorAvatar, EntryFrame } from "@/activity/entry-frame"
import { ShowFilter, TimelineFilters } from "@/activity/show-filter"
import {
  DayBreakRow,
  useStickToBottom,
  useToday,
} from "@/activity/timeline-list"
import { threadKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Composer } from "@/comments/composer"
import {
  agentMentionsIn,
  mentionedAgents,
  threadParticipants,
} from "@/comments/mentions"
import { MessageText } from "@/comments/message-text"
import { RunStarted } from "@/comments/run-started"
import {
  useOpenThread,
  useReplyToThread,
  useThreads,
  type Message,
  askedAgent,
  type SentMessage,
} from "@/comments/thread-queries"
import { useMentionableAgents } from "@/comments/use-mentionable-agents"
import { liveRegionText } from "@/lib/live-region"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import {
  discussionEntries,
  discussionModel,
  type DiscussionChange,
  type DiscussionShow,
} from "./discussion-model"

const showOptions: { value: DiscussionShow; label: string }[] = [
  { value: "everything", label: "Everything" },
  { value: "messages", label: "Messages only" },
  { value: "changes", label: "Changes only" },
]

type DiscussionFeedProps = {
  target: string
  hint: string
  placeholder?: string
  emptyText: string
  changes?: {
    entries: readonly DiscussionChange[]
    onSelect: (subjectId: string) => void
  }
  messageActions?: (message: Message) => ReactNode
}

type DiscussionProps = Omit<DiscussionFeedProps, "changes"> & {
  className?: string
}

function LoadingRows() {
  return (
    <div aria-busy="true" className="flex flex-col gap-4 px-4 py-3">
      {[0, 1, 2].map((row) => (
        <div key={row} className="flex gap-2.5">
          <Skeleton className="size-6 flex-none rounded-full" />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-3.5 w-24" />
            <Skeleton className="h-3.5 w-full" />
          </div>
        </div>
      ))}
    </div>
  )
}

export function DiscussionFeed({
  target,
  hint,
  placeholder,
  emptyText,
  changes,
  messageActions,
}: DiscussionFeedProps) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const {
    threads,
    canComment,
    isPending,
    isError,
    error,
    refetch,
    isFetching,
  } = useThreads(target)
  const openThread = useOpenThread(target)
  const replyToThread = useReplyToThread(target)
  const agents = useMentionableAgents()
  const [sent, setSent] = useState<{
    result: SentMessage
    agentName: string
  } | null>(null)
  const [show, setShow] = useState<DiscussionShow>("everything")
  const today = useToday()
  const { messages, replyThreadId, canWrite } = useMemo(
    () => discussionModel(threads, canComment),
    [threads, canComment]
  )
  const people = useMemo(
    () => threadParticipants(messages.map((message) => message.actor)),
    [messages]
  )
  const entries = useMemo(
    () => discussionEntries(messages, changes?.entries ?? [], show),
    [messages, changes, show]
  )
  const stickRef = useStickToBottom(
    `${entries.length}:${entries.at(-1)?.key ?? ""}`
  )

  const seen = useRef<Set<string> | null>(null)
  const [announcement, setAnnouncement] = useState({ text: "", count: 0 })
  useEffect(() => {
    if (isPending || isError) return
    if (seen.current === null) {
      seen.current = new Set(messages.map((message) => message.id))
      return
    }
    const known = seen.current
    const fresh = messages.filter((message) => !known.has(message.id))
    for (const message of fresh) known.add(message.id)
    const others = fresh.filter((message) => message.actor.id !== principal.id)
    if (others.length === 0) return
    setAnnouncement((previous) => ({
      text: others
        .map((message) => `New message from ${message.actor.label}`)
        .join(". "),
      count: previous.count + 1,
    }))
  }, [isPending, isError, messages, principal.id])

  async function send(body: string) {
    const startRuns = agentMentionsIn(body) === 1
    setSent(null)
    const result =
      replyThreadId !== null
        ? await replyToThread.mutateAsync({
            threadId: replyThreadId,
            body,
            startRuns,
          })
        : await openThread.mutateAsync({ body, startRuns })
    if (askedAgent(result)) {
      setSent({ result, agentName: mentionedAgents(body)[0]?.name ?? "" })
    }
  }

  let disabledReason: string | null = null
  if (isPending) disabledReason = "Loading the discussion"
  else if (isError) disabledReason = "The discussion is not loaded"
  else if (!canWrite) {
    disabledReason =
      "You can read this discussion. People who can comment write here."
  }

  let body: ReactNode
  if (isPending) {
    body = <LoadingRows />
  } else if (isError) {
    body = (
      <div className="px-4 py-3">
        <LoadProblem
          title="Could not load the discussion"
          detail={sentence(error instanceof Error ? error.message : "")}
          retrying={isFetching}
          onRetry={() => void refetch()}
        />
      </div>
    )
  } else if (entries.length === 0) {
    body =
      show === "everything" ? (
        <p className="px-4 py-3 text-[13px] text-muted-foreground">
          {emptyText}
        </p>
      ) : (
        <div className="px-4 py-8 text-center text-[13px] text-muted-foreground">
          <p>No entries match these filters.</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => setShow("everything")}
          >
            Clear filters
          </Button>
        </div>
      )
  } else {
    body = (
      <ol aria-label="Discussion" className="flex flex-col px-4 pt-1 pb-4">
        {withDayBreaks(entries, today).map((row) => {
          if (row.kind === "day") {
            return <DayBreakRow key={row.key} label={row.label} />
          }
          const { entry } = row
          if (entry.kind === "message") {
            const { message } = entry
            const actions = messageActions?.(message)
            return (
              <EntryFrame
                key={entry.key}
                avatar={actorAvatar(message.actor)}
                who={message.actor.label}
                verb={entry.verb}
                at={message.created_at}
                edited={message.edited_at !== null}
              >
                <MessageText
                  message={message}
                  people={people}
                  queryKey={threadKeys.list(target)}
                />
                {actions ? (
                  <span className="flex flex-wrap items-center gap-0.5">
                    {actions}
                  </span>
                ) : null}
              </EntryFrame>
            )
          }
          const { change } = entry
          const { subject } = change
          return (
            <EntryFrame
              key={entry.key}
              avatar={actorAvatar(change.actor)}
              who={change.actor.label}
              verb={change.verb}
              at={change.at}
              panel={
                subject === null
                  ? undefined
                  : {
                      title: subject.title,
                      onSelect: () => changes?.onSelect(subject.id),
                    }
              }
            />
          )
        })}
      </ol>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {changes === undefined ? null : (
        <TimelineFilters>
          <ShowFilter options={showOptions} value={show} onChange={setShow} />
        </TimelineFilters>
      )}
      <div ref={stickRef} className="min-h-0 flex-1 overflow-y-auto">
        {body}
      </div>
      <div className="border-t px-4 pt-3 pb-4">
        {sent === null ? null : (
          <div className="pb-2">
            <RunStarted
              run={sent.result.run}
              refusal={sent.result.runRefusal}
              agentName={sent.agentName}
            />
          </div>
        )}
        <Composer
          label="Message"
          placeholder={placeholder ?? "Message, or type @ to ask an agent"}
          hint={hint}
          people={people}
          agents={agents}
          disabledReason={disabledReason}
          onSubmit={send}
        />
      </div>
      <p role="status" className="sr-only">
        {liveRegionText(announcement.text, announcement.count)}
      </p>
    </div>
  )
}

export function Discussion({ className, ...feed }: DiscussionProps) {
  return (
    <aside
      aria-label="Discussion"
      className={cn(
        "flex min-h-0 flex-col rounded-card border border-border bg-card",
        className
      )}
    >
      <header className="flex items-center justify-between gap-2 border-b border-border px-4 py-2.5">
        <h2 className="text-[13px] font-semibold">Discussion</h2>
      </header>
      <DiscussionFeed {...feed} />
    </aside>
  )
}
