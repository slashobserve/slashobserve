import { useQuery, useQueryClient } from "@tanstack/react-query"

import type { components } from "@/api/schema.gen"

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

export const starterNote =
  "This starter dashboard is kept current by SlashObserve releases, so it cannot be edited."

export const fileOwnedNote =
  "This dashboard comes from a file on the server. The next change to that file replaces edits made here, which stay in the version history."

export function editNote(
  dashboard: Pick<components["schemas"]["DashboardSummary"], "managed_by"> & {
    permissions: Pick<DashboardPermissions, "edit">
  }
): string | null {
  return dashboard.managed_by === "file" && dashboard.permissions.edit
    ? fileOwnedNote
    : null
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
      return `${starterNote} Duplicate it from the dashboard menu to get a copy you can change.`
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

function selectEditNote(envelope: DashboardEnvelope) {
  return editNote(envelope.dashboard)
}

export function useLatestEditNote(dashboardId: string): string | null {
  const queryClient = useQueryClient()
  return (
    useQuery({
      ...latestDashboardQuery(queryClient, dashboardId),
      staleTime: Infinity,
      select: selectEditNote,
    }).data ?? null
  )
}

function selectChange(envelope: DashboardEnvelope) {
  const { permissions, provisioned } = envelope.dashboard
  return { content: contentChange(permissions, provisioned), provisioned }
}

export function useLatestChange(dashboardId: string | null) {
  const queryClient = useQueryClient()
  return useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled: dashboardId !== null,
    staleTime: Infinity,
    select: selectChange,
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
