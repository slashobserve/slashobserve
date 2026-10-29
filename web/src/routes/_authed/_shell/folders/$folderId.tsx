import { createFileRoute } from "@tanstack/react-router"

import { FolderSettings } from "@/folders/folder-settings"

export const Route = createFileRoute("/_authed/_shell/folders/$folderId")({
  staticData: {
    crumbs: [
      { label: "Dashboards", to: "/dashboards" },
      { label: "Folder settings" },
    ],
  },
  component: FolderPage,
})

function FolderPage() {
  const { folderId } = Route.useParams()
  return <FolderSettings key={folderId} folderId={folderId} />
}
