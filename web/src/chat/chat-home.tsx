import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { useDatasources } from "@/datasources/queries"
import { DashboardsSection } from "@/home/dashboards-section"
import { useAgentInfo } from "@/observe/queries"
import { SavedPrompts } from "@/prompts/saved-prompts"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

import { ChatComposer, useOpenSent, useSendExample } from "./chat-composer"
import { examplePrompts } from "./examples"
import { RecentChats } from "./recent-chats"

const noDashboards: readonly { id: string; title: string }[] = []

export function ChatHome() {
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const dashboards = useQuery(dashboardTitlesQuery())
  const { send, pending } = useSendExample()
  const openSent = useOpenSent()

  const examples = examplePrompts({
    types: (datasources.data ?? []).filter((d) => d.healthy).map((d) => d.type),
    dashboards: dashboards.data ?? noDashboards,
  })
  const agentReady = !agent.isPending && agent.data?.available !== false
  const agentOff = agent.data?.available === false

  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3.5 pt-10 pb-20">
      <h1 className="mb-1.5 text-center text-[26px] font-semibold tracking-[-0.01em]">
        What do you want to see?
      </h1>
      {agentOff ? <DashboardsSection /> : null}
      <div className="w-full max-w-[720px]">
        <ChatComposer
          rows={3}
          placeholder="Describe a dashboard, like: checkout health by pod, last 6h"
          autoFocus
          promptPicker
          onSent={openSent}
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
              key={example.label}
              type="button"
              variant="outline"
              className="font-normal text-muted-foreground"
              disabled={pending}
              onClick={() => void send(example.prompt, example.dashboardId)}
            >
              {example.label}
            </Button>
          ))}
        </div>
      )}
      {agentReady ? <DashboardsSection /> : null}
      <SavedPrompts />
      <RecentChats />
    </div>
  )
}
