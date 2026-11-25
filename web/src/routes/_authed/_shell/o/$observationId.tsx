import { createFileRoute } from "@tanstack/react-router"

import { threadParam } from "@/discussion/thread-param"
import { useRecordRecent } from "@/home/recent"
import { ObservationPage } from "@/observations/observation-page"
import { useObservation } from "@/observations/queries"

type ObservationSearch = { rca?: "1"; thread?: string }

export const Route = createFileRoute("/_authed/_shell/o/$observationId")({
  staticData: {
    crumbs: [
      { label: "Observations", to: "/observations" },
      { label: "Observation" },
    ],
  },
  validateSearch: (search: Record<string, unknown>): ObservationSearch => ({
    rca: search.rca === "1" ? "1" : undefined,
    thread: threadParam(search.thread),
  }),
  component: ObservationRoute,
})

function ObservationRoute() {
  const { observationId } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const title = useObservation(observationId).data?.observation.title
  useRecordRecent("observation", observationId, title)
  return (
    <ObservationPage
      key={observationId}
      observationId={observationId}
      focusedThreadId={search.thread}
      onClearThread={() =>
        void navigate({
          search: (prev) => ({ ...prev, thread: undefined }),
          replace: true,
        })
      }
      rca={{
        open: search.rca === "1",
        onOpenChange: (open) =>
          void navigate({
            search: (prev) => ({ ...prev, rca: open ? "1" : undefined }),
            replace: !open,
          }),
      }}
    />
  )
}
