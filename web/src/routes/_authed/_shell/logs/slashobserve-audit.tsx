import { createFileRoute } from "@tanstack/react-router"

import { auditDatasourceUid } from "@/audit/audit-source"
import { RoleGate } from "@/components/role-gate"
import { LogsPage } from "@/logs/logs-page"
import { preloadLogsSource } from "@/logs/preload-source"
import { validateLogsSearch } from "@/logs/search"
import { roleAtLeast } from "@/lib/roles"

export const Route = createFileRoute("/_authed/_shell/logs/slashobserve-audit")(
  {
    validateSearch: validateLogsSearch,
    staticData: {
      crumbs: [{ label: "Audit", to: "/audit" }, { label: "Explore" }],
      pageTitle: true,
    },
    loader: ({ context }) =>
      roleAtLeast(context.principal.role, "admin")
        ? preloadLogsSource(context.queryClient)
        : undefined,
    component: AuditExploreRoute,
  }
)

function AuditExploreRoute() {
  return (
    <RoleGate min="admin">
      <LogsPage uid={auditDatasourceUid} />
    </RoleGate>
  )
}
