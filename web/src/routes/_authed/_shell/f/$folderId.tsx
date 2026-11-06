import { useCallback } from "react"
import { createFileRoute } from "@tanstack/react-router"

import { validateListSearch, type ListSearch } from "@/dashboards/list-filters"
import { FolderPage } from "@/folders/folder-page"

type FolderSearch = ListSearch & { folder?: undefined; settings?: "1" }

function validateFolderSearch(raw: Record<string, unknown>): FolderSearch {
  return {
    ...validateListSearch(raw),
    folder: undefined,
    settings: raw.settings === "1" ? "1" : undefined,
  }
}

export const Route = createFileRoute("/_authed/_shell/f/$folderId")({
  staticData: {
    crumbs: [{ label: "Dashboards", to: "/dashboards" }, { label: "Folder" }],
  },
  validateSearch: validateFolderSearch,
  component: FolderRoute,
})

function FolderRoute() {
  const { folderId } = Route.useParams()
  const { settings, ...search } = Route.useSearch()
  const navigate = Route.useNavigate()
  const onSearchChange = useCallback(
    (next: ListSearch) =>
      void navigate({
        search: (prev) => ({
          ...next,
          folder: undefined,
          settings: prev.settings,
        }),
        replace: true,
      }),
    [navigate]
  )
  const onSettings = useCallback(
    (open: boolean) =>
      void navigate({
        search: (prev) => ({ ...prev, settings: open ? "1" : undefined }),
        replace: true,
      }),
    [navigate]
  )
  return (
    <FolderPage
      key={folderId}
      folderId={folderId}
      search={search}
      onSearchChange={onSearchChange}
      settingsOpen={settings === "1"}
      onSettings={onSettings}
    />
  )
}
