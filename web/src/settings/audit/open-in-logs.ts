import { logsLink } from "@/lib/links"
import { validQuery } from "@/logs/search"

import type { AuditView } from "./search"

export const auditDatasourceUid = "slashobserve-audit"

export function auditInLogsLink(view: Pick<AuditView, "q" | "from" | "to">) {
  return logsLink(auditDatasourceUid, {
    q: validQuery(view.q),
    from: view.from,
    to: view.to,
  })
}
