import { useMemo } from "react"

import { EntryFrame } from "@/activity/entry-frame"
import { threadKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { Skeleton } from "@/components/ui/skeleton"
import { threadParticipants } from "@/comments/mentions"
import { MessageText } from "@/comments/message-text"
import { useThreads, type ThreadWithMessages } from "@/comments/thread-queries"
import { Composer } from "@/conversation/composer"
import { threadPlace, ungated } from "@/conversation/delivery"
import {
  answerCoversBody,
  RunAnswer,
  shownAnswer,
} from "@/conversation/run-answer"
import { subjectOf } from "@/conversation/send-plan"
import { useComposer } from "@/conversation/use-composer"
import { markOf } from "@/lib/author"
import { sentence } from "@/observe/queries"

export function ThreadDetail({
  target,
  threadId,
}: {
  target: string
  threadId: string
}) {
  const {
    threads,
    canComment,
    isPending,
    isError,
    error,
    refetch,
    isFetching,
  } = useThreads(target)
  const thread = threads.find((entry) => entry.thread.id === threadId)
  const messages = useMemo(() => thread?.messages ?? [], [thread])
  const people = useMemo(
    () => threadParticipants(messages.map((message) => message.actor)),
    [messages]
  )

  if (isError && thread === undefined) {
    return (
      <LoadProblem
        title="Could not load this thread"
        detail={sentence(error?.message ?? "")}
        retrying={isFetching}
        onRetry={() => void refetch()}
      />
    )
  }
  if (isPending) {
    return (
      <div aria-busy="true" className="flex flex-col gap-3">
        <Skeleton className="h-10 rounded-card" />
        <Skeleton className="h-10 rounded-card" />
      </div>
    )
  }
  if (thread === undefined) {
    return (
      <p className="text-[13px] text-muted-foreground">
        This thread was deleted or you can no longer see it.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-3">
      <ol aria-label="Thread" className="flex flex-col">
        {messages.map((message) => {
          const answer = shownAnswer(message, thread.thread)
          return (
            <EntryFrame
              key={message.id}
              nested
              avatar={markOf(message.actor)}
              who={message.actor.label}
              verb={message.id === threadId ? "wrote" : "replied"}
              at={message.created_at}
              edited={message.edited_at !== null}
            >
              <MessageText
                message={message}
                people={people}
                target={target}
                queryKey={threadKeys.list(target)}
                root={message.id === threadId}
                bodyHidden={answerCoversBody(message, answer)}
              />
              {answer === null ? null : (
                <RunAnswer
                  answer={answer}
                  target={target}
                  thread={thread.thread}
                  gate={{ kind: "allowed" }}
                />
              )}
            </EntryFrame>
          )
        })}
      </ol>
      {canComment ? (
        <ThreadReply target={target} thread={thread} people={people} />
      ) : null}
    </div>
  )
}

function ThreadReply({
  target,
  thread,
  people,
}: {
  target: string
  thread: ThreadWithMessages
  people: ReturnType<typeof threadParticipants>
}) {
  const model = useComposer({
    subject: subjectOf(target),
    place: threadPlace(thread.thread.id, thread.thread),
    gate: ungated,
    people,
    onSent: () => undefined,
  })
  return (
    <Composer
      model={model}
      layout="docked"
      label="Reply"
      formLabel="Reply to this thread"
      placeholder="Reply in this thread"
    />
  )
}
