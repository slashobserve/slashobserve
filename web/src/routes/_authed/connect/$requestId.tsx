import { createFileRoute } from "@tanstack/react-router"

import { ConsentPage } from "@/connect/consent"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/connect/$requestId")({
  component: ConnectRoute,
})

function ConnectRoute() {
  useDocumentTitle("Connect an agent - SlashObserve")
  const { requestId } = Route.useParams()
  return <ConsentPage key={requestId} requestId={requestId} />
}
