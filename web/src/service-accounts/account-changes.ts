import type { Role } from "@/lib/roles"

import type { ServiceAccount } from "./queries"

export type AccountChanges = { role?: Role; takes_runs?: boolean }

export function accountChanges(
  account: Pick<ServiceAccount, "role" | "takes_runs">,
  wanted: { role: Role; takesRuns: boolean }
): AccountChanges | null {
  const changes: AccountChanges = {}
  if (wanted.role !== account.role) changes.role = wanted.role
  if (wanted.takesRuns !== account.takes_runs) {
    changes.takes_runs = wanted.takesRuns
  }
  return Object.keys(changes).length === 0 ? null : changes
}
