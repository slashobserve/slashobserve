import type { ListSearch } from "@/dashboards/list-filters"
import { FolderPage } from "@/folders/folder-page"

import { AuditFolderGate } from "./audit-folder-gate"
import { auditPageTitles } from "./audit-folder"

export function AuditDashboards({
  search,
  onSearchChange,
  settingsOpen,
  onSettings,
}: {
  search: ListSearch
  onSearchChange: (next: ListSearch) => void
  settingsOpen: boolean
  onSettings: (open: boolean) => void
}) {
  return (
    <AuditFolderGate title="Dashboards">
      {(folder) => (
        <FolderPage
          key={folder.id}
          folderId={folder.id}
          search={search}
          onSearchChange={onSearchChange}
          settingsOpen={settingsOpen}
          onSettings={onSettings}
          dashboardsOnly={{
            heading: "Dashboards",
            documentTitle: auditPageTitles.dashboards,
          }}
        />
      )}
    </AuditFolderGate>
  )
}
