import { useMemo, useState } from "react"

import { EntryFrame } from "@/activity/entry-frame"
import { threadKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
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
  askedAgent,
  useReplyToThread,
  useThreads,
  type SentMessage,
} from "@/comments/thread-queries"
import { useMentionableAgents } from "@/comments/use-mentionable-agents"
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
  const reply = useReplyToThread(target)
  const agents = useMentionableAgents()
  const [started, setStarted] = useState<{
    result: SentMessage
    agentName: string
  } | null>(null)
  const thread = threads.find((entry) => entry.thread.id === threadId)
  const messages = useMemo(() => thread?.messages ?? [], [thread])
  const people = useMemo(
    () => threadParticipants(messages.map((message) => message.actor)),
    [messages]
  )

  async function send(body: string) {
    setStarted(null)
    const result = await reply.mutateAsync({
      threadId,
      body,
      startRuns: agentMentionsIn(body) === 1,
    })
    if (askedAgent(result)) {
      setStarted({ result, agentName: mentionedAgents(body)[0]?.name ?? "" })
    }
  }

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
        {messages.map((message) => (
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
            />
          </EntryFrame>
        ))}
      </ol>
      {canComment ? (
        <Composer
          label="Reply"
          formLabel="Reply to this thread"
          placeholder="Reply, or type @ to ask an agent"
          people={people}
          target={target}
          agents={agents}
          onSubmit={send}
        />
      ) : null}
      {started === null ? null : (
        <RunStarted
          run={started.result.run}
          refusal={started.result.runRefusal}
          agentName={started.agentName}
        />
      )}
    </div>
  )
}
