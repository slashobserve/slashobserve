import { createFileRoute } from "@tanstack/react-router"
import { useCallback } from "react"

import { useDocumentTitle } from "@/lib/document-title"
import { ObservationList } from "@/observations/observation-list"
import {
  searchOf,
  validateObservationsSearch,
  viewOf,
  type ObservationListView,
} from "@/observations/list-view"

export const Route = createFileRoute("/_authed/_shell/observations/")({
  staticData: { crumbs: [{ label: "Observations" }] },
  validateSearch: validateObservationsSearch,
  component: ObservationsRoute,
})

function ObservationsRoute() {
  useDocumentTitle("Observations - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const go = useCallback(
    (next: ObservationListView, creating: boolean) => {
      void navigate({ search: searchOf(next, creating) })
    },
    [navigate]
  )
  return (
    <ObservationList
      view={viewOf(search)}
      creating={search.new === "1"}
      onChange={go}
    />
  )
}
