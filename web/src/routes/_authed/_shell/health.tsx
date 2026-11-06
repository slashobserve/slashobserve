import { createFileRoute } from "@tanstack/react-router"

import { HealthPage } from "@/health/health-page"
import { isHealthFilter, type HealthFilter } from "@/health/health-labels"
import { useDocumentTitle } from "@/lib/document-title"

type HealthSearch = { kind?: HealthFilter; folder?: string }

export const Route = createFileRoute("/_authed/_shell/health")({
  staticData: { crumbs: [{ label: "Health" }] },
  validateSearch: (search: Record<string, unknown>): HealthSearch => ({
    kind: isHealthFilter(search.kind) ? search.kind : undefined,
    folder:
      typeof search.folder === "string" && search.folder !== ""
        ? search.folder
        : undefined,
  }),
  component: HealthRoute,
})

function HealthRoute() {
  useDocumentTitle("Dashboard health - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <div className="mx-auto w-full max-w-6xl">
      <HealthPage
        kind={search.kind ?? null}
        folderId={search.folder ?? null}
        onFilters={({ kind, folderId }) =>
          void navigate({
            search: {
              ...(kind === null ? {} : { kind }),
              ...(folderId === null ? {} : { folder: folderId }),
            },
          })
        }
      />
    </div>
  )
}
