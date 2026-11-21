import { useQuery, useQueryClient } from "@tanstack/react-query"

import type { components } from "@/api/schema.gen"
import { roleAtLeast, type Role } from "@/lib/roles"

import { latestDashboardQuery, type DashboardEnvelope } from "./queries"

export type DashboardPermissions = components["schemas"]["Permissions"]
export type DashboardAccess = Pick<
  components["schemas"]["DashboardSummary"],
  "folder" | "permissions" | "provisioned"
>
const noPermissions: DashboardPermissions = {
  comment: false,
  edit: false,
  manage: false,
  create: false,
}
export function mayInvestigate(
  p: DashboardPermissions,
  role: Role,
  provisioned: boolean
): boolean {
  return mayEditBesidesContent(p, provisioned) && roleAtLeast(role, "editor")
}

export type ContentChange = "edit" | "propose" | "none"

export function contentChange(
  p: Pick<DashboardPermissions, "comment" | "edit">,
  provisioned: boolean
): ContentChange {
  if (provisioned) return "none"
  if (p.edit) return "edit"
  if (p.comment) return "propose"
  return "none"
}

export function mayEditBesidesContent(
  p: Pick<DashboardPermissions, "edit">,
  provisioned: boolean
): boolean {
  return p.edit || provisioned
}

export type ComposerMode = "full" | "comment-only" | "provisioned" | "read-only"

export function composerMode(
  p: DashboardPermissions,
  provisioned: boolean
): ComposerMode {
  if (provisioned) return p.comment ? "provisioned" : "read-only"
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
      return `You can view and comment in ${folderTitle}. What the agent changes for you reaches the dashboard as a proposal that someone with Edit on this folder merges.`
    case "provisioned":
      return "This starter dashboard is kept current by SlashObserve releases, so it cannot be edited. Duplicate it from the dashboard menu to get a copy you can change."
    case "read-only":
      return `You can view dashboards in ${folderTitle}. To comment, ask the people who manage this folder for Comment access.`
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

function selectProvisioned(envelope: DashboardEnvelope) {
  return envelope.dashboard.provisioned
}

export function useLatestProvisioned(dashboardId: string): boolean {
  const queryClient = useQueryClient()
  return (
    useQuery({
      ...latestDashboardQuery(queryClient, dashboardId),
      staleTime: Infinity,
      select: selectProvisioned,
    }).data ?? false
  )
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
