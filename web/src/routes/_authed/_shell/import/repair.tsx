import { createFileRoute } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { RepairPage } from "@/import/repair/repair-page"
import { useDocumentTitle } from "@/lib/document-title"

type RepairSearch = { folder?: string }

export const Route = createFileRoute("/_authed/_shell/import/repair")({
  staticData: {
    crumbs: [
      { label: "Observe", to: "/" },
      { label: "Import from Grafana" },
      { label: "Repair queue" },
    ],
  },
  validateSearch: (search: Record<string, unknown>): RepairSearch => ({
    folder:
      typeof search.folder === "string" && search.folder !== ""
        ? search.folder
        : undefined,
  }),
  component: RepairRoute,
})

function RepairRoute() {
  useDocumentTitle("Repair queue - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <RoleGate min="editor">
      <div className="mx-auto w-full max-w-6xl">
        <RepairPage
          folderId={search.folder ?? null}
          onFolder={(folder) =>
            void navigate({ search: folder === null ? {} : { folder } })
          }
        />
      </div>
    </RoleGate>
  )
}
