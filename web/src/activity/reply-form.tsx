import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Composer } from "@/comments/composer"
import {
  agentMentionsIn,
  mentionedAgents,
  type MentionPerson,
} from "@/comments/mentions"
import { RunStarted } from "@/comments/run-started"
import {
  askedAgent,
  useReplyToThread,
  type SentMessage,
} from "@/comments/thread-queries"
import { useMentionableAgents } from "@/comments/use-mentionable-agents"

import type { AgentGate } from "./agent-gate"

type Props = {
  dashboardId: string
  rootId: string
  people: readonly MentionPerson[]
  gate: AgentGate
  latestVersion: number
  onClose: () => void
}

export function ReplyForm({
  dashboardId,
  rootId,
  people,
  gate,
  onClose,
}: Props) {
  const reply = useReplyToThread(`dashboard:${dashboardId}`)
  const agents = useMentionableAgents()
  const [started, setStarted] = useState<{
    result: SentMessage
    agentName: string
  } | null>(null)

  async function send(body: string) {
    setStarted(null)
    const result = await reply.mutateAsync({
      threadId: rootId,
      body,
      startRuns: agentMentionsIn(body) === 1,
    })
    if (askedAgent(result)) {
      setStarted({ result, agentName: mentionedAgents(body)[0]?.name ?? "" })
    } else {
      onClose()
    }
  }

  return (
    <div className="mt-1 flex flex-col gap-2">
      <Composer
        label="Reply"
        formLabel="Reply to this comment"
        placeholder="Reply, or type @ to ask an agent"
        people={people}
        agents={agents}
        agentsBlocked={gate.kind === "blocked" ? gate.reason : null}
        autoFocus
        onSubmit={send}
        onCancel={onClose}
      />
      {gate.kind === "blocked" ? (
        <p className="text-xs text-muted-foreground">{gate.reason}</p>
      ) : null}
      {started === null ? null : (
        <RunStarted
          run={started.result.run}
          refusal={started.result.runRefusal}
          agentName={started.agentName}
        />
      )}
      <div className="flex justify-end">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-disabled={reply.isPending || undefined}
          onClick={() => {
            if (!reply.isPending) onClose()
          }}
        >
          {started === null ? "Cancel" : "Done"}
        </Button>
      </div>
    </div>
  )
}
