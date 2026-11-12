import { createFileRoute } from "@tanstack/react-router"

import { ObservationPage } from "@/observations/observation-page"

type ObservationSearch = { rca?: "1" }

export const Route = createFileRoute("/_authed/_shell/o/$observationId")({
  staticData: {
    crumbs: [
      { label: "Observations", to: "/observations" },
      { label: "Observation" },
    ],
  },
  validateSearch: (search: Record<string, unknown>): ObservationSearch => ({
    rca: search.rca === "1" ? "1" : undefined,
  }),
  component: ObservationRoute,
})

function ObservationRoute() {
  const { observationId } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ObservationPage
      key={observationId}
      observationId={observationId}
      rca={{
        open: search.rca === "1",
        onOpenChange: (open) =>
          void navigate({
            search: open ? { rca: "1" } : {},
            replace: !open,
          }),
      }}
    />
  )
}
