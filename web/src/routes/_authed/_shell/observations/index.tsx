import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import {
  ObservationList,
  type ObservationStateFilter,
} from "@/observations/observation-list"

type ObservationsSearch = { state?: ObservationStateFilter; new?: "1" }

export const Route = createFileRoute("/_authed/_shell/observations/")({
  staticData: { crumbs: [{ label: "Observations" }] },
  validateSearch: (search: Record<string, unknown>): ObservationsSearch => ({
    state:
      search.state === "resolved" || search.state === "all"
        ? search.state
        : undefined,
    new: search.new === "1" ? "1" : undefined,
  }),
  component: ObservationsRoute,
})

function ObservationsRoute() {
  useDocumentTitle("Observations - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const state = search.state ?? "open"
  function go(next: { state: ObservationStateFilter; creating: boolean }) {
    void navigate({
      search: {
        ...(next.state === "open" ? {} : { state: next.state }),
        ...(next.creating ? { new: "1" as const } : {}),
      },
    })
  }
  return (
    <div className="mx-auto w-full max-w-6xl">
      <ObservationList
        state={state}
        creating={search.new === "1"}
        onChange={go}
      />
    </div>
  )
}
