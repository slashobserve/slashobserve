import { createFileRoute } from "@tanstack/react-router"

import { parseSearchKind, type SearchKind } from "@/search/queries"
import { SearchPage } from "@/search/search-page"

type SearchParams = { q?: string | undefined; kind?: SearchKind | undefined }

export const Route = createFileRoute("/_authed/_shell/search")({
  staticData: { crumbs: [{ label: "Search" }] },
  validateSearch: (search: Record<string, unknown>): SearchParams => ({
    q:
      typeof search.q === "string" && search.q.trim() !== ""
        ? search.q.trim()
        : undefined,
    kind: parseSearchKind(search.kind),
  }),
  component: SearchRoute,
})

function SearchRoute() {
  const { q, kind } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SearchPage
      q={q ?? ""}
      kind={kind ?? null}
      onChange={(next) =>
        void navigate({
          search: {
            q: next.q === "" ? undefined : next.q,
            kind: next.kind ?? undefined,
          },
        })
      }
    />
  )
}
