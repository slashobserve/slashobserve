import { createFileRoute, redirect } from "@tanstack/react-router"

import { homeDashboardHref } from "@/home/home-dashboard"
import { HomePage } from "@/home/home-page"
import {
  homeRedirect,
  homeSearch,
  parseHomeTab,
  type HomeRouteSearch,
} from "@/home/home-tab"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/_chat/")({
  validateSearch: (search: Record<string, unknown>): HomeRouteSearch =>
    homeSearch(parseHomeTab(search.tab)),
  beforeLoad: async ({ location, context }) => {
    const href = homeRedirect(location.searchStr)
    if (href !== null) throw redirect({ href, replace: true })
    const home = await homeDashboardHref(
      context.queryClient,
      location.searchStr
    )
    if (home !== null) throw redirect({ href: home, replace: true })
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
