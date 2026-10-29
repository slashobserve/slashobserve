import { Outlet, createFileRoute, useParams } from "@tanstack/react-router"

import { ChatHistory } from "@/chat/chat-history"
import { PromptPanelHost } from "@/prompts/prompt-panel-host"

export const Route = createFileRoute("/_authed/_shell/_chat")({
  component: ChatLayout,
})

function ChatLayout() {
  const { requestId } = useParams({ strict: false })
  return (
    <>
      <Outlet />
      <ChatHistory currentRequestId={requestId} />
      <PromptPanelHost />
    </>
  )
}
