import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useRouteContext } from "@tanstack/react-router"
import { PanelRightCloseIcon } from "lucide-react"

import { threadKeys } from "@/api/keys"
import { AuthorMark } from "@/components/author-mark"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime, useNow } from "@/components/relative-time"
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
import { markOf } from "@/lib/author"
import { formatDate } from "@/lib/date-format"
import { liveRegionText } from "@/lib/live-region"
import { formatDateTime } from "@/lib/time"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { discussionModel } from "./discussion-model"

type DiscussionProps = {
  target: string
  hint: string
  placeholder?: string
  emptyText: string
  onHide?: () => void
  messageActions?: (message: Message) => ReactNode
  className?: string
}

const bottomSlack = 24

function isAtBottom(list: HTMLElement): boolean {
  return list.scrollHeight - list.scrollTop - list.clientHeight <= bottomSlack
}

function MessageTime({ iso }: { iso: string }) {
  const now = useNow()
  const ms = Date.parse(iso)
  if (new Date(ms).toDateString() !== new Date(now).toDateString()) {
    return <RelativeTime iso={iso} />
  }
  return (
    <time dateTime={iso} title={formatDateTime(iso)}>
      {formatDate(ms, "HH:mm")}
    </time>
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

export function Discussion({
  target,
  hint,
  placeholder,
  emptyText,
  onHide,
  messageActions,
  className,
}: DiscussionProps) {
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
  const { messages, replyThreadId, canWrite } = useMemo(
    () => discussionModel(threads, canComment),
    [threads, canComment]
  )
  const people = useMemo(
    () => threadParticipants(messages.map((message) => message.actor)),
    [messages]
  )

  const list = useRef<HTMLOListElement>(null)
  const following = useRef(true)

  useLayoutEffect(() => {
    const element = list.current
    if (element !== null && following.current) {
      element.scrollTop = element.scrollHeight
    }
  }, [messages.length])

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
    following.current = true
    const element = list.current
    if (element !== null) element.scrollTop = element.scrollHeight
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
  } else if (messages.length === 0) {
    body = (
      <p className="px-4 py-3 text-[13px] text-muted-foreground">{emptyText}</p>
    )
  } else {
    body = (
      <ol
        ref={list}
        aria-label="Messages"
        className="min-h-0 flex-1 overflow-auto py-1"
        onScroll={(event) => {
          following.current = isAtBottom(event.currentTarget)
        }}
      >
        {messages.map((message) => (
          <li key={message.id} className="flex gap-2.5 px-4 py-2.5">
            <AuthorMark mark={markOf(message.actor)} size={24} />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex items-baseline gap-2">
                <span className="text-[13px] font-semibold">
                  {message.actor.label}
                </span>
                <span className="text-xs text-muted-foreground">
                  <MessageTime iso={message.created_at} />
                </span>
              </div>
              <MessageText
                message={message}
                people={people}
                queryKey={threadKeys.list(target)}
              />
              {messageActions?.(message)}
            </div>
          </li>
        ))}
      </ol>
    )
  }

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
        {onHide === undefined ? null : (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Hide discussion"
            onClick={onHide}
          >
            <PanelRightCloseIcon aria-hidden="true" />
          </Button>
        )}
      </header>
      <div className="flex min-h-0 flex-1 flex-col">{body}</div>
      <div className="p-4 pt-2">
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
    </aside>
  )
}
