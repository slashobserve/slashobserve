import { createFileRoute, redirect } from "@tanstack/react-router"

import { HomePage } from "@/home/home-page"
import {
  homeRedirect,
  homeSearch,
  parseHomeTab,
  type HomeRouteSearch,
} from "@/home/home-tab"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/_chat/")({
  staticData: { crumbs: [{ label: "Home" }] },
  validateSearch: (search: Record<string, unknown>): HomeRouteSearch =>
    homeSearch(parseHomeTab(search.tab)),
  beforeLoad: ({ location }) => {
    const href = homeRedirect(location.searchStr)
    if (href !== null) throw redirect({ href, replace: true })
  },
  component: HomeRoute,
})

function HomeRoute() {
  useDocumentTitle("Home - SlashObserve")
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <HomePage
      tab={parseHomeTab(tab)}
      onTabChange={(next) =>
        void navigate({ search: homeSearch(next), replace: true })
      }
    />
  )
}
