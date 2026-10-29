import { useNavigate, useRouteContext } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { useDatasources } from "@/datasources/queries"
import { useAgentInfo } from "@/observe/queries"
import type { ObserveRequest } from "@/observe/queries"
import { canDriveAgent } from "@/prompts/access"
import { SavedPrompts } from "@/prompts/saved-prompts"

import { ChatComposer, useSendExample } from "./chat-composer"
import { examplePrompts } from "./examples"
import { RecentChats } from "./recent-chats"

export function ChatHome() {
  const navigate = useNavigate()
  const { principal } = useRouteContext({ from: "/_authed" })
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const { send, pending } = useSendExample()

  const examples = examplePrompts(
    (datasources.data ?? []).filter((d) => d.healthy).map((d) => d.type)
  )
  const agentReady = !agent.isPending && agent.data?.available !== false

  function open(request: ObserveRequest) {
    return navigate({
      to: "/observe/$requestId",
      params: { requestId: request.id },
    })
  }

  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3.5 pt-10 pb-20">
      <h1 className="mb-1.5 text-center text-[26px] font-semibold tracking-[-0.01em]">
        What do you want to see?
      </h1>
      <div className="w-full max-w-[720px]">
        <ChatComposer
          rows={3}
          placeholder="Describe a dashboard, like: checkout health by pod, last 6h"
          autoFocus
          promptPicker={canDriveAgent(principal)}
          onSent={open}
        />
      </div>
      {!agentReady || examples.length === 0 ? null : (
        <div
          role="group"
          aria-label="Example prompts"
          className="flex max-w-[720px] flex-wrap justify-center gap-2"
        >
          {examples.map((example) => (
            <Button
              key={example}
              type="button"
              variant="outline"
              className="font-normal text-muted-foreground"
              disabled={pending}
              onClick={() => void send(example)}
            >
              {example}
            </Button>
          ))}
        </div>
      )}
      <SavedPrompts />
      <RecentChats />
    </div>
  )
}
