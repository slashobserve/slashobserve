import { useCallback, useRef, useState } from "react"
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  ReplyIcon,
  RotateCcwIcon,
} from "lucide-react"

import { useSetCommentStatus } from "@/comments/queries"
import { cn } from "@/lib/utils"

import type { AgentGate } from "./agent-gate"
import {
  commentAvatar,
  EntryAction,
  EntryBody,
  EntryFrame,
  EntryMeta,
  MetaState,
  RequestFailure,
  ResolvedState,
  SentToAgent,
} from "./entry-frame"
import { ReplyForm } from "./reply-form"
import type { TimelineEntry } from "./timeline-model"
import { canResolveThread, threadState, threadVerbText } from "./thread-view"

type Props = {
  dashboardId: string
  entry: Extract<TimelineEntry, { kind: "thread" }>
  panelTitle: string | null
  onSelectPanel: (panelId: string) => void
  gate: AgentGate
  canComment: boolean
  latestVersion: number
  clientName: string
}

function replyCount(count: number): string {
  return count === 1 ? "1 reply" : `${count} replies`
}

export function ThreadEntry({
  dashboardId,
  entry,
  panelTitle,
  onSelectPanel,
  gate,
  canComment,
  latestVersion,
  clientName,
}: Props) {
  const { thread, request, panelId, askedAgent } = entry
  const { root, replies } = thread
  const [replying, setReplying] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const replyButton = useRef<HTMLButtonElement>(null)
  const statusButton = useRef<HTMLButtonElement | null>(null)
  const toggleButton = useRef<HTMLButtonElement>(null)
  const setStatus = useSetCommentStatus(dashboardId)
  const resolved = root.status === "resolved"
  const folded = resolved && !expanded
  const verb = threadVerbText(askedAgent, panelId, panelTitle)
  const panel =
    panelId !== null && panelTitle !== null
      ? { title: panelTitle, onSelect: () => onSelectPanel(panelId) }
      : undefined

  const statusRef = useCallback((node: HTMLButtonElement | null) => {
    statusButton.current = node
    if (node === null) return
    return () => {
      statusButton.current = null
      if (document.activeElement === node) replyButton.current?.focus()
    }
  }, [])

  function closeReply() {
    setReplying(false)
    replyButton.current?.focus()
  }

  function openReply() {
    if (folded) setExpanded(true)
    setReplying(true)
  }

  function toggleStatus() {
    if (setStatus.isPending) return
    const status = resolved ? "open" : "resolved"
    setStatus.mutate(
      { commentId: root.id, status },
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
          setReplying(false)
          if (focusedOnStatus) toggleButton.current?.focus()
        },
      }
    )
  }

  const view = threadState({ root, request, gate, clientName })
  let state
  switch (view.kind) {
    case "sent":
      state = <SentToAgent />
      break
    case "failed":
      state = <RequestFailure error={view.error} />
      break
    case "resolved":
      state = <ResolvedState text={view.text} />
      break
    case "note":
      state = <MetaState>{view.text}</MetaState>
      break
  }

  return (
    <EntryFrame
      avatar={commentAvatar(root)}
      who={root.actor.label}
      verb={verb}
      panel={panel}
      at={root.created_at}
    >
      <EntryBody className={cn(folded && "line-clamp-1 text-muted-foreground")}>
        {root.body}
      </EntryBody>
      {folded || replies.length === 0 ? null : (
        <ol className="mt-1 flex flex-col">
          {replies.map((reply) => (
            <EntryFrame
              key={reply.id}
              nested
              avatar={commentAvatar(reply)}
              who={reply.actor.label}
              verb="replied"
              at={reply.created_at}
            >
              <EntryBody>{reply.body}</EntryBody>
            </EntryFrame>
          ))}
        </ol>
      )}
      {canComment && replying && !folded ? (
        <ReplyForm
          dashboardId={dashboardId}
          rootId={root.id}
          gate={gate}
          latestVersion={latestVersion}
          onClose={closeReply}
        />
      ) : null}
      <EntryMeta>
        {state}
        {folded && replies.length > 0 ? (
          <span>{replyCount(replies.length)}</span>
        ) : null}
        <span className="flex flex-wrap items-center gap-0.5">
          {canComment ? (
            <EntryAction
              ref={replyButton}
              icon={<ReplyIcon aria-hidden="true" />}
              aria-expanded={replying && !folded}
              onClick={openReply}
            >
              Reply
            </EntryAction>
          ) : null}
          {canComment && canResolveThread(request) ? (
            <EntryAction
              ref={statusRef}
              icon={
                resolved ? (
                  <RotateCcwIcon aria-hidden="true" />
                ) : (
                  <CheckIcon aria-hidden="true" />
                )
              }
              aria-disabled={setStatus.isPending || undefined}
              aria-busy={setStatus.isPending || undefined}
              onClick={toggleStatus}
            >
              {resolved ? "Reopen" : "Resolve"}
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
