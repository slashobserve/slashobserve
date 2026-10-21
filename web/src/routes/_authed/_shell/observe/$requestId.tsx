import { createFileRoute } from "@tanstack/react-router"

import { ChatHistory } from "@/chat/chat-history"
import { Conversation } from "@/chat/conversation"

export const Route = createFileRoute("/_authed/_shell/observe/$requestId")({
  staticData: { crumbs: [{ label: "Observe" }] },
  component: ObserveRoute,
})

function ObserveRoute() {
  const { requestId } = Route.useParams()
  return (
    <>
      <Conversation key={requestId} requestId={requestId} />
      <ChatHistory currentRequestId={requestId} />
    </>
  )
}
