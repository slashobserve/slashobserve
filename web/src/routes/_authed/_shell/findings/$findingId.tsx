import { createFileRoute } from "@tanstack/react-router"

import { FindingPage } from "@/findings/finding-page"

export const Route = createFileRoute("/_authed/_shell/findings/$findingId")({
  staticData: {
    crumbs: [{ label: "Observe", to: "/" }, { label: "Finding" }],
  },
  component: FindingRoute,
})

function FindingRoute() {
  const { findingId } = Route.useParams()
  return <FindingPage key={findingId} findingId={findingId} />
}
