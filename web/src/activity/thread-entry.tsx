import { useRef, useState, type ComponentProps } from "react"

import { useSetCommentStatus } from "@/comments/queries"
import { cn } from "@/lib/utils"
import { isActive } from "@/revisions/queries"

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

function threadVerb(
  panelId: string | null,
  panelTitle: string | null,
  onSelectPanel: (panelId: string) => void
): { verb: string; panel?: { title: string; onSelect: () => void } } {
  if (panelId === null) return { verb: "commented" }
  if (panelTitle === null) return { verb: "commented on a removed panel" }
  return {
    verb: "commented on",
    panel: { title: panelTitle, onSelect: () => onSelectPanel(panelId) },
  }
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
  const { thread, request, panelId } = entry
  const { root, replies } = thread
  const [replying, setReplying] = useState(false)
  const replyButton = useRef<HTMLButtonElement>(null)
  const setStatus = useSetCommentStatus(dashboardId)
  const resolved = root.status === "resolved"
  const { verb, panel } = threadVerb(panelId, panelTitle, onSelectPanel)

  function closeReply() {
    setReplying(false)
    replyButton.current?.focus()
  }

  let state
  if (request !== null && isActive(request.status)) {
    state = <SentToAgent />
  } else if (request?.status === "failed" && !resolved) {
    state = <RequestFailure error={request.error} />
  } else if (resolved && root.resolved_by_version !== null) {
    state = (
      <ResolvedState text={`Resolved by version ${root.resolved_by_version}`} />
    )
  } else if (resolved) {
    state = (
      <ResolvedState
        text={`Resolved by ${root.resolved_by?.name ?? "someone"}`}
      />
    )
  } else if (gate.kind === "no-model") {
    state = <MetaState>Open, waiting for {clientName}</MetaState>
  } else {
    state = <MetaState>Open</MetaState>
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
