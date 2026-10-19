import { useRef, useState, type ComponentProps } from "react"

import { useSetCommentStatus } from "@/comments/queries"
import { cn } from "@/lib/utils"

import type { AgentGate } from "./agent-gate"
import {
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
  latestVersion: number
  clientName: string
}

function MetaButton({ className, ...props }: ComponentProps<"button">) {
  return (
    <button
      type="button"
      className={cn(
        "rounded-sm underline outline-none hover:text-foreground focus-visible:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
}

export function ThreadEntry({
  dashboardId,
  entry,
  panelTitle,
  onSelectPanel,
  gate,
  latestVersion,
  clientName,
}: Props) {
  const { thread, request, panelId, askedAgent } = entry
  const { root, replies } = thread
  const [replying, setReplying] = useState(false)
  const replyButton = useRef<HTMLButtonElement>(null)
  const setStatus = useSetCommentStatus(dashboardId)
  const resolved = root.status === "resolved"
  const verb = threadVerbText(askedAgent, panelId, panelTitle)
  const panel =
    panelId !== null && panelTitle !== null
      ? { title: panelTitle, onSelect: () => onSelectPanel(panelId) }
      : undefined

  function closeReply() {
    setReplying(false)
    replyButton.current?.focus()
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
      avatar={{ kind: "person", name: root.author.name }}
      who={root.author.name}
      verb={verb}
      panel={panel}
      at={root.created_at}
    >
      <EntryBody>{root.body}</EntryBody>
      <EntryMeta>
        {state}
        <MetaButton
          ref={replyButton}
          aria-expanded={replying}
          onClick={() => setReplying(true)}
        >
          Reply
        </MetaButton>
        {canResolveThread(request) ? (
          <MetaButton
            aria-disabled={setStatus.isPending || undefined}
            aria-busy={setStatus.isPending || undefined}
            onClick={() => {
              if (setStatus.isPending) return
              setStatus.mutate({
                commentId: root.id,
                status: resolved ? "open" : "resolved",
              })
            }}
          >
            {resolved ? "Reopen" : "Resolve"}
          </MetaButton>
        ) : null}
      </EntryMeta>
      {replies.length === 0 ? null : (
        <ol className="mt-1 flex flex-col border-l pl-3">
          {replies.map((reply) => (
            <EntryFrame
              key={reply.id}
              nested
              avatar={{ kind: "person", name: reply.author.name }}
              who={reply.author.name}
              verb="replied"
              at={reply.created_at}
            >
              <EntryBody>{reply.body}</EntryBody>
            </EntryFrame>
          ))}
        </ol>
      )}
      {replying ? (
        <ReplyForm
          dashboardId={dashboardId}
          rootId={root.id}
          gate={gate}
          latestVersion={latestVersion}
          onClose={closeReply}
        />
      ) : null}
    </EntryFrame>
  )
}
