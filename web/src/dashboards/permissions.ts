import { useQuery, useQueryClient } from "@tanstack/react-query"

import type { components } from "@/api/schema.gen"

import { latestDashboardQuery, type DashboardEnvelope } from "./queries"

export type DashboardPermissions = components["schemas"]["Permissions"]
export type DashboardAccess = Pick<
  components["schemas"]["DashboardSummary"],
  "folder" | "permissions"
>
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

export function readerNote(
  mode: ComposerMode,
  folderTitle: string
): string | null {
  switch (mode) {
    case "full":
      return null
    case "comment-only":
      return `You can view and comment in ${folderTitle}. Someone with Edit on this folder can send your comment to the agent or change the panels.`
    case "read-only":
      return `You can view dashboards in ${folderTitle}. Ask an Admin if you need to comment.`
  }
}

function selectPermissions(envelope: DashboardEnvelope) {
  return envelope.dashboard.permissions
}

function selectFolderTitle(envelope: DashboardEnvelope) {
  return envelope.dashboard.folder.title
}

export function useLatestFolderTitle(dashboardId: string): string | undefined {
  const queryClient = useQueryClient()
  return useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select: selectFolderTitle,
  }).data
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
