import { createFileRoute } from "@tanstack/react-router"

import { BuildPage } from "@/observe/build-page"

export const Route = createFileRoute("/_authed/builds/$requestId")({
  component: BuildRoute,
})

function BuildRoute() {
  const { requestId } = Route.useParams()
  return <BuildPage key={requestId} requestId={requestId} />
}
