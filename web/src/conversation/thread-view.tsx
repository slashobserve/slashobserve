import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  ReplyIcon,
  RotateCcwIcon,
} from "lucide-react"

import {
  EntryAction,
  EntryFrame,
  EntryMeta,
  MetaState,
  ResolvedState,
} from "@/activity/entry-frame"
import { threadKeys } from "@/api/keys"
import { MessageText } from "@/comments/message-text"
import { threadParticipants, type MentionPerson } from "@/comments/mentions"
import {
  useMarkThreadRead,
  useSetThreadStatus,
  type Message,
  type ThreadWithMessages,
} from "@/comments/thread-queries"
import { markOf } from "@/lib/author"
import { cn } from "@/lib/utils"

import type { AgentGate } from "./delivery"
import { RunAnswer, RunLine, RunLink } from "./run-answer"
import {
  panelText,
  replyCountText,
  threadStatus,
  unansweredRun,
} from "./thread-state"

const foldMs = 200

export const arrivingClass =
  "motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-bottom-1 motion-safe:animation-duration-200 motion-safe:ease-out"

function MessageRow({
  message,
  entry,
  target,
  people,
  arriving,
  gate,
}: {
  message: Message
  entry: ThreadWithMessages
  target: string
  people: readonly MentionPerson[]
  arriving: boolean
  gate: AgentGate
}) {
  const answer = message.answer ?? null
  const builtinRunId = message.actor.run_id ?? null
  const pending = unansweredRun(message, entry.messages)
  return (
    <EntryFrame
      nested
      avatar={markOf(message.actor)}
      who={message.actor.label}
      at={message.created_at}
      edited={message.edited_at !== null}
      className={arriving ? arrivingClass : undefined}
    >
      <MessageText
        message={message}
        people={people}
        target={target}
        queryKey={threadKeys.list(target)}
      />
      {builtinRunId === null ? null : (
        <p className="text-xs text-muted-foreground">
          <RunLink runId={builtinRunId} />
        </p>
      )}
      {answer === null ? null : (
        <RunAnswer
          answer={answer}
          target={target}
          thread={entry.thread}
          gate={gate}
        />
      )}
      {pending === null ? null : <RunLine message={message} run={pending} />}
    </EntryFrame>
  )
}

function Fold({ open, children }: { open: boolean; children: ReactNode }) {
  const [mounted, setMounted] = useState(open)
  if (open && !mounted) setMounted(true)
  useEffect(() => {
    if (open) return
    const timer = window.setTimeout(() => setMounted(false), foldMs)
    return () => window.clearTimeout(timer)
  }, [open])
  return (
    <div
      inert={!open}
      className={cn(
        "grid motion-safe:transition-[grid-template-rows,opacity] motion-safe:duration-200 motion-safe:ease-out",
        open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
      )}
    >
      <div className="min-h-0 overflow-hidden">{mounted ? children : null}</div>
    </div>
  )
}

type Props = {
  entry: ThreadWithMessages
  target: string
  panelTitle: string | null
  onSelectPanel?: (panelId: string) => void
  focused?: boolean
  gate: AgentGate
  clientName: string
  onReplyHere: () => void
}

export function ThreadView({
  entry,
  target,
  panelTitle,
  onSelectPanel,
  focused,
  gate,
  clientName,
  onReplyHere,
}: Props) {
  const { thread, messages } = entry
  const root = messages[0]
  const replies = messages.slice(1)
  const [expanded, setExpanded] = useState(false)
  const [firstSeen] = useState(
    () => new Set(messages.map((message) => message.id))
  )
  const replyButton = useRef<HTMLButtonElement>(null)
  const statusButton = useRef<HTMLButtonElement | null>(null)
  const toggleButton = useRef<HTMLButtonElement>(null)
  const setStatus = useSetThreadStatus(target)
  const markRead = useMarkThreadRead(target)
  const reading = useRef(false)
  const people = threadParticipants(messages.map((message) => message.actor))
  const allowed = thread.allowed_actions
  const canReply = allowed.includes("reply")
  const canResolve = allowed.includes("resolve")
  const canReopen = allowed.includes("reopen")
  const resolved = thread.status === "resolved"
  const folded = resolved && !expanded
  const panelId = thread.panel_id
  const panel =
    panelId !== null && panelTitle !== null
      ? {
          title: panelTitle,
          onSelect: onSelectPanel && (() => onSelectPanel(panelId)),
        }
      : undefined

  const statusRef = useCallback((node: HTMLButtonElement | null) => {
    statusButton.current = node
    if (node === null) return
    return () => {
      statusButton.current = null
      if (document.activeElement === node) replyButton.current?.focus()
    }
  }, [])

  if (root === undefined) return null

  function readThread() {
    if (!thread.unread || reading.current) return
    reading.current = true
    markRead.mutate(thread.id, {
      onSettled: () => {
        reading.current = false
      },
    })
  }

  function replyHere() {
    if (folded) setExpanded(true)
    onReplyHere()
  }

  function toggleStatus() {
    if (setStatus.isPending) return
    const status = canReopen ? "open" : "resolved"
    setStatus.mutate(
      { threadId: thread.id, status },
      {
        onSuccess: () => {
          if (status === "open") {
            setExpanded(true)
            return
          }
          const focusedOnStatus =
            document.activeElement !== null &&
            document.activeElement === statusButton.current
          setExpanded(false)
          if (focusedOnStatus) toggleButton.current?.focus()
        },
      }
    )
  }

  const status = threadStatus(thread, gate, clientName)
  const pending = unansweredRun(root, messages)

  return (
    <EntryFrame
      avatar={markOf(root.actor)}
      who={root.actor.label}
      verb={panelText(panelId, panelTitle)}
      panel={panel}
      at={root.created_at}
      edited={root.edited_at !== null}
      focused={focused}
      unread={thread.unread ? { onRead: readThread } : undefined}
    >
      <MessageText
        message={root}
        people={people}
        target={target}
        queryKey={threadKeys.list(target)}
        root
        folded={folded}
      />
      {folded || pending === null ? null : (
        <RunLine message={root} run={pending} />
      )}
      {replies.length === 0 ? null : (
        <Fold open={!folded}>
          <ol className="mt-1 flex flex-col">
            {replies.map((reply) => (
              <MessageRow
                key={reply.id}
                message={reply}
                entry={entry}
                target={target}
                people={people}
                arriving={!firstSeen.has(reply.id)}
                gate={gate}
              />
            ))}
          </ol>
        </Fold>
      )}
      <EntryMeta>
        {status.kind === "resolved" ? (
          <ResolvedState text={status.text} />
        ) : (
          <MetaState>{status.text}</MetaState>
        )}
        {folded && replies.length > 0 ? (
          <span>{replyCountText(replies.length)}</span>
        ) : null}
        <span className="flex flex-wrap items-center gap-0.5">
          {canReply ? (
            <EntryAction
              ref={replyButton}
              icon={<ReplyIcon aria-hidden="true" />}
              onClick={replyHere}
            >
              Reply
            </EntryAction>
          ) : null}
          {canResolve || canReopen ? (
            <EntryAction
              ref={statusRef}
              icon={
                canReopen ? (
                  <RotateCcwIcon aria-hidden="true" />
                ) : (
                  <CheckIcon aria-hidden="true" />
                )
              }
              aria-disabled={setStatus.isPending || undefined}
              aria-busy={setStatus.isPending || undefined}
              onClick={toggleStatus}
            >
              {canReopen ? "Reopen" : "Resolve"}
            </EntryAction>
          ) : null}
          {resolved ? (
            <EntryAction
              ref={toggleButton}
              icon={
                folded ? (
                  <ChevronDownIcon aria-hidden="true" />
                ) : (
                  <ChevronUpIcon aria-hidden="true" />
                )
              }
              aria-expanded={!folded}
              onClick={() => setExpanded(folded)}
            >
              {folded ? "Show thread" : "Hide thread"}
            </EntryAction>
          ) : null}
        </span>
      </EntryMeta>
    </EntryFrame>
  )
}
