import { createFileRoute } from "@tanstack/react-router"

import { RequestProgress } from "@/observe/request-progress"

export const Route = createFileRoute("/_authed/_shell/observe/$requestId")({
  staticData: { crumbs: [{ label: "Observe" }] },
  component: ObserveRoute,
})

function ObserveRoute() {
  const { requestId } = Route.useParams()
  return <RequestProgress key={requestId} requestId={requestId} />
}
