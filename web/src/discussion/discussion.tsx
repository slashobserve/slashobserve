import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { useRouteContext } from "@tanstack/react-router"
import { XIcon } from "lucide-react"

import { withDayBreaks } from "@/activity/day-label"
import { EntryAction, EntryFrame, EntryMeta } from "@/activity/entry-frame"
import {
  DayBreakRow,
  useStickToBottom,
  useToday,
} from "@/activity/timeline-list"
import { threadKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { ScrollPane } from "@/components/scroll-pane"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { threadParticipants } from "@/comments/mentions"
import { MessageText } from "@/comments/message-text"
import { useThreads, type Message } from "@/comments/thread-queries"
import { Composer } from "@/conversation/composer"
import { threadPlace, ungated } from "@/conversation/delivery"
import { RunAnswer, RunLine } from "@/conversation/run-answer"
import { subjectOf } from "@/conversation/send-plan"
import { unansweredRun } from "@/conversation/thread-state"
import { useComposer } from "@/conversation/use-composer"
import { markOf } from "@/lib/author"
import { liveRegionText } from "@/lib/live-region"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import {
  filterEntries,
  isFiltered,
  noFilter,
  type BlockRef,
  type DiscussionFilter,
} from "./discussion-filter"
import {
  discussionEntries,
  discussionModel,
  quotedText,
  type DiscussionChange,
} from "./discussion-model"

type DiscussionFeedProps = {
  target: string
  hint: string
  placeholder?: string
  emptyText: string
  changes?: {
    entries: readonly DiscussionChange[]
    onSelect: (subjectId: string) => void
  }
  blocks?: {
    items: readonly BlockRef[]
    anchor: string | null
    onAnchorChange: (blockId: string | null) => void
    onShow: (blockId: string) => void
  }
  view?: { filter: DiscussionFilter; onClear: () => void }
  messageActions?: (
    message: Message,
    tools: { reply: (() => void) | null }
  ) => ReactNode
  readOnlyAction?: ReactNode
}

type DiscussionProps = Omit<
  DiscussionFeedProps,
  "changes" | "blocks" | "view" | "readOnlyAction"
> & {
  className?: string
}

const noBlock = "none"

export function BlockSelect({
  id,
  label,
  noneLabel,
  optionPrefix = "",
  blocks,
  value,
  onChange,
  className,
}: {
  id?: string
  label: string
  noneLabel: string
  optionPrefix?: string
  blocks: readonly BlockRef[]
  value: string | null
  onChange: (blockId: string | null) => void
  className?: string
}) {
  const items = [
    { value: noBlock, label: noneLabel },
    ...blocks.map((block) => ({
      value: block.id,
      label: `${optionPrefix}${block.title}`,
    })),
  ]
  return (
    <Select
      items={items}
      value={value ?? noBlock}
      onValueChange={(next) => {
        if (next !== null) onChange(next === noBlock ? null : next)
      }}
    >
      <SelectTrigger id={id} size="sm" aria-label={label} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
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
  blocks,
  view,
  messageActions,
  readOnlyAction,
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
  const [replyingTo, setReplyingTo] = useState<Message | null>(null)
  const filter = view?.filter ?? noFilter
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
    () =>
      filterEntries(
        discussionEntries(messages, changes?.entries ?? []),
        filter
      ),
    [messages, changes, filter]
  )
  const anchor =
    blocks?.items.find((block) => block.id === blocks.anchor)?.id ?? null
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

  const model = useComposer({
    subject: subjectOf(target),
    place: threadPlace(
      replyThreadId,
      threads.find((entry) => entry.thread.id === replyThreadId)?.thread
    ),
    target: anchor === null ? undefined : { blockId: anchor },
    gate: { ...ungated, canComment: canWrite },
    people,
    replyToId: replyingTo?.id,
    onSent: () => setReplyingTo(null),
  })

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
    body = !isFiltered(filter) ? (
      <p className="px-4 py-3 text-[13px] text-muted-foreground">{emptyText}</p>
    ) : (
      <div className="px-4 py-8 text-center text-[13px] text-muted-foreground">
        <p>No entries match these filters.</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={view?.onClear}
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
            const actions = messageActions?.(message, {
              reply:
                disabledReason === null ? () => setReplyingTo(message) : null,
            })
            const answer = message.answer ?? null
            const answerThread = threads.find(
              (item) => item.thread.id === message.thread_id
            )?.thread
            const waitingOn = unansweredRun(message, messages)
            const block =
              message.block_id === null
                ? undefined
                : blocks?.items.find((item) => item.id === message.block_id)
            return (
              <EntryFrame
                key={entry.key}
                avatar={markOf(message.actor)}
                who={message.actor.label}
                verb={entry.verb}
                at={message.created_at}
                edited={message.edited_at !== null}
              >
                {message.reply_to === null ? null : (
                  <p className="border-l-2 pl-2 text-[13px] wrap-anywhere text-muted-foreground">
                    {quotedText(message.reply_to.excerpt)}
                  </p>
                )}
                <MessageText
                  message={message}
                  people={people}
                  target={target}
                  queryKey={threadKeys.list(target)}
                  root={message.id === message.thread_id}
                />
                {answer === null || answerThread === undefined ? null : (
                  <RunAnswer
                    answer={answer}
                    target={target}
                    thread={answerThread}
                    gate={{ kind: "allowed" }}
                  />
                )}
                {waitingOn === null ? null : (
                  <RunLine message={message} run={waitingOn} />
                )}
                {block === undefined ? null : (
                  <EntryMeta>
                    <span>On {block.title}</span>
                    <EntryAction
                      icon={null}
                      aria-label={`Show ${block.title}`}
                      onClick={() => blocks?.onShow(block.id)}
                    >
                      Show
                    </EntryAction>
                  </EntryMeta>
                )}
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
              avatar={markOf(change.actor)}
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
            >
              {change.detail === null ? null : (
                <EntryMeta>{change.detail}</EntryMeta>
              )}
            </EntryFrame>
          )
        })}
      </ol>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ScrollPane ref={stickRef} className="flex-1">
        {body}
      </ScrollPane>
      <div className="border-t px-4 pt-3 pb-4">
        {isPending ||
        isError ||
        canWrite ||
        readOnlyAction === undefined ? null : (
          <div className="flex flex-col pb-2">{readOnlyAction}</div>
        )}
        {replyingTo === null ? null : (
          <div className="flex items-center gap-1 pb-2 text-xs text-muted-foreground">
            <span className="min-w-0 truncate">
              Replying to {replyingTo.actor.label}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Stop replying"
              onClick={() => setReplyingTo(null)}
            >
              <XIcon aria-hidden="true" />
            </Button>
          </div>
        )}
        <Composer
          model={model}
          layout="docked"
          label="Message"
          placeholder={placeholder ?? "Message, or type @ to ask an agent"}
          hint={hint}
          leading={
            blocks === undefined ? undefined : (
              <BlockSelect
                label="Write on"
                noneLabel="On the whole page"
                optionPrefix="On "
                blocks={blocks.items}
                value={anchor}
                onChange={blocks.onAnchorChange}
                className="max-w-full min-w-0"
              />
            )
          }
          disabledReason={disabledReason}
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
