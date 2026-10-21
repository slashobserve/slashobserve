import { createFileRoute } from "@tanstack/react-router"

import { Conversation } from "@/chat/conversation"

export const Route = createFileRoute("/_authed/_shell/observe/$requestId")({
  staticData: { crumbs: [{ label: "Observe" }] },
  component: ObserveRoute,
})

function ObserveRoute() {
  const { requestId } = Route.useParams()
  return <Conversation key={requestId} requestId={requestId} />
}
