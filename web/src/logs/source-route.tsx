import type { QueryClient } from "@tanstack/react-query"

import { auditDatasourceUid } from "@/audit/audit-source"
import { RoleGate } from "@/components/role-gate"
import { datasourcesQuery } from "@/datasources/queries"
import { roleAtLeast, type Role } from "@/lib/roles"
import { agentInfoQuery } from "@/observe/queries"
import type { Crumb } from "@/shell/crumbs"

import { LogsPage } from "./logs-page"

export function LogsSource({ uid }: { uid: string }) {
  if (uid !== auditDatasourceUid) return <LogsPage uid={uid} />
  return (
    <RoleGate min="admin">
      <LogsPage uid={uid} />
    </RoleGate>
  )
}

export function logsSourceCrumbs(params: { datasourceUid?: string }): Crumb[] {
  return params.datasourceUid === auditDatasourceUid
    ? [{ label: "Audit", to: "/audit" }, { label: "Explore" }]
    : [{ label: "Logs", to: "/logs" }]
}

export async function preloadLogsSource(
  queryClient: QueryClient,
  role: Role,
  uid: string
) {
  if (uid === auditDatasourceUid && !roleAtLeast(role, "admin")) return
  await Promise.allSettled([
    queryClient.ensureQueryData(datasourcesQuery()),
    queryClient.ensureQueryData(agentInfoQuery()),
  ])
}
