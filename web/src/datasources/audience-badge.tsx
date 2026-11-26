import { Badge } from "@/components/ui/badge"

import type { DatasourceStatus } from "./queries"

export function AudienceBadge({
  datasource,
}: {
  datasource: Pick<DatasourceStatus, "audience">
}) {
  if (datasource.audience !== "admins") return null
  return <Badge variant="outline">Admins only</Badge>
}
