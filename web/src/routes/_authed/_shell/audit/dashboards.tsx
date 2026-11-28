import { useCallback } from "react"
import { createFileRoute } from "@tanstack/react-router"

import { AuditDashboards } from "@/audit/audit-dashboards"
import { auditPageTitles } from "@/audit/audit-folder"
import { RoleGate } from "@/components/role-gate"
import { validateListSearch, type ListSearch } from "@/dashboards/list-filters"
import { useDocumentTitle } from "@/lib/document-title"

type AuditDashboardsSearch = Omit<ListSearch, "folder"> & {
  folder?: undefined
  settings?: "1" | undefined
}

function validateAuditDashboardsSearch(
  raw: Record<string, unknown>
): AuditDashboardsSearch {
  return {
    ...validateListSearch(raw),
    folder: undefined,
    settings: raw.settings === "1" ? "1" : undefined,
  }
}

export const Route = createFileRoute("/_authed/_shell/audit/dashboards")({
  staticData: {
    crumbs: [{ label: "Audit", to: "/audit" }, { label: "Dashboards" }],
  },
  validateSearch: validateAuditDashboardsSearch,
  component: AuditDashboardsRoute,
})

function AuditDashboardsRoute() {
  useDocumentTitle(auditPageTitles.dashboards)
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
    <RoleGate min="admin" hideCrumb>
      <AuditDashboards
        search={search}
        onSearchChange={onSearchChange}
        settingsOpen={settings === "1"}
        onSettings={onSettings}
      />
    </RoleGate>
  )
}
