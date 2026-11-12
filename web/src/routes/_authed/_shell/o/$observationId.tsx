import { createFileRoute } from "@tanstack/react-router"

import { ObservationPage } from "@/observations/observation-page"

export const Route = createFileRoute("/_authed/_shell/o/$observationId")({
  staticData: {
    crumbs: [
      { label: "Observations", to: "/observations" },
      { label: "Observation" },
    ],
  },
  component: ObservationRoute,
})

function ObservationRoute() {
  const { observationId } = Route.useParams()
  return <ObservationPage key={observationId} observationId={observationId} />
}
