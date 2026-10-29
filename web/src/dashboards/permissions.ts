import { useQuery, useQueryClient } from "@tanstack/react-query"

import type { components } from "@/api/schema.gen"
import { roleLabels, type Role } from "@/lib/roles"

import { latestDashboardQuery, type DashboardEnvelope } from "./queries"

export type DashboardPermissions = components["schemas"]["Permissions"]
const noPermissions: DashboardPermissions = {
  comment: false,
  edit: false,
  manage: false,
  create: false,
}
export type ComposerMode = "full" | "comment-only" | "read-only"

export function composerMode(p: DashboardPermissions): ComposerMode {
  if (p.edit) return "full"
  if (p.comment) return "comment-only"
  return "read-only"
}

export function readerNote(mode: ComposerMode, role: Role): string | null {
  switch (mode) {
    case "full":
      return null
    case "comment-only":
      return `You can view and comment as a ${roleLabels[role]}. An Editor can send your comment to the agent or change the panels.`
    case "read-only":
      return `You can view this dashboard as a ${roleLabels[role]}. Ask an Admin if you need to comment.`
  }
}

function selectPermissions(envelope: DashboardEnvelope) {
  return envelope.dashboard.permissions
}

export function useLatestPermissions(
  dashboardId: string
): DashboardPermissions {
  const queryClient = useQueryClient()
  return (
    useQuery({
      ...latestDashboardQuery(queryClient, dashboardId),
      staleTime: Infinity,
      select: selectPermissions,
    }).data ?? noPermissions
  )
}
