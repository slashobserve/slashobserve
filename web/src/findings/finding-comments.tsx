import { useState } from "react"
import { CheckIcon, ReplyIcon, RotateCcwIcon } from "lucide-react"

import {
  avatarFor,
  EntryAction,
  EntryFrame,
  ResolvedState,
} from "@/activity/entry-frame"
import { ApiError } from "@/api/errors"
import { threadKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { MessageForm } from "@/comments/message-form"
import { MessageText } from "@/comments/message-text"
import { threadParticipants, type MentionPerson } from "@/comments/mentions"
import {
  useOpenThread,
  useReplyToThread,
  useSetThreadStatus,
  useThreads,
  type Message,
  type ThreadWithMessages,
} from "@/comments/thread-queries"
import { Skeleton } from "@/components/ui/skeleton"
import { isAgentActor } from "@/lib/origin"
import { sentence } from "@/observe/queries"

function avatarOf(message: Message) {
  return isAgentActor(message.actor)
    ? { kind: "agent" as const }
    : avatarFor(message.actor)
}

function ThreadView({
  entry,
  target,
  canComment,
  people,
}: {
  entry: ThreadWithMessages
  target: string
  canComment: boolean
  people: readonly MentionPerson[]
}) {
  const { thread, messages } = entry
  const [replying, setReplying] = useState(false)
  const reply = useReplyToThread(target)
  const setStatus = useSetThreadStatus(target)
  const resolved = thread.status === "resolved"
  const queryKey = threadKeys.list(target)

  return (
    <li className="rounded-card border px-3 py-1">
      <ol aria-label="Comment thread">
        {messages.map((message, index) => (
          <EntryFrame
            key={message.id}
            nested
            avatar={avatarOf(message)}
            who={message.actor.label}
            verb={index === 0 ? "commented" : "replied"}
            at={message.created_at}
            edited={message.edited_at !== null}
          >
            <MessageText
              message={message}
              people={people}
              queryKey={queryKey}
            />
          </EntryFrame>
        ))}
      </ol>
      {canComment && replying ? (
        <div className="pb-2">
          <MessageForm
            label="Reply"
            submitLabel="Reply"
            people={people}
            autoFocus
            onSubmit={async (body) => {
              await reply.mutateAsync({ threadId: thread.id, body })
              setReplying(false)
            }}
            onCancel={() => setReplying(false)}
          />
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pb-1 text-xs text-muted-foreground">
        {resolved ? (
          <ResolvedState
            text={`Resolved by ${thread.resolved_actor?.label ?? "someone"}`}
          />
        ) : (
          <span>Open</span>
        )}
        {canComment ? (
          <span className="flex flex-wrap items-center gap-0.5">
            <EntryAction
              icon={<ReplyIcon aria-hidden="true" />}
              aria-expanded={replying}
              onClick={() => setReplying(true)}
            >
              Reply
            </EntryAction>
            <EntryAction
              icon={
                resolved ? (
                  <RotateCcwIcon aria-hidden="true" />
                ) : (
                  <CheckIcon aria-hidden="true" />
                )
              }
              aria-disabled={setStatus.isPending || undefined}
              aria-busy={setStatus.isPending || undefined}
              onClick={() => {
                if (setStatus.isPending) return
                setStatus.mutate({
                  threadId: thread.id,
                  status: resolved ? "open" : "resolved",
                })
              }}
            >
              {resolved ? "Reopen" : "Resolve"}
            </EntryAction>
          </span>
        ) : null}
      </div>
    </li>
  )
}

export function FindingComments({ findingId }: { findingId: string }) {
  const target = `finding:${findingId}`
  const {
    threads,
    canComment,
    isPending,
    isError,
    error,
    isFetching,
    refetch,
  } = useThreads(target)
  const open = useOpenThread(target)
  const people = threadParticipants(
    threads.flatMap((entry) => entry.messages.map((message) => message.actor))
  )

  return (
    <section aria-labelledby="finding-comments-title" className="mt-6">
      <h2 id="finding-comments-title" className="mb-2 text-sm font-semibold">
        Comments
      </h2>
      {isPending ? (
        <Skeleton aria-busy="true" className="h-16 rounded-card" />
      ) : isError ? (
        <LoadProblem
          title="Could not load the comments"
          detail={
            error instanceof ApiError
              ? sentence(error.message)
              : "Check your connection and try again."
          }
          retrying={isFetching}
          onRetry={() => void refetch()}
        />
      ) : (
        <div className="flex flex-col gap-3">
          {threads.length === 0 ? (
            <p className="text-[13px] text-muted-foreground">
              No comments yet.
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {threads.map((entry) => (
                <ThreadView
                  key={entry.thread.id}
                  entry={entry}
                  target={target}
                  canComment={canComment}
                  people={people}
                />
              ))}
            </ul>
          )}
          {canComment ? (
            <MessageForm
              label="Comment on this finding"
              submitLabel="Comment"
              placeholder="Write a comment"
              people={people}
              onSubmit={(body) => open.mutateAsync({ body })}
            />
          ) : null}
        </div>
      )}
    </section>
  )
}
