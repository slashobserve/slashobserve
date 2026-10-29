import type { components } from "@/api/schema.gen"

export type Role = components["schemas"]["Role"]

export const roles: readonly Role[] = ["admin", "editor", "commenter", "viewer"]

export function roleAtLeast(role: Role, min: Role): boolean {
  return roles.indexOf(role) <= roles.indexOf(min)
}

export const roleLabels: Record<Role, string> = {
  admin: "Admin",
  editor: "Editor",
  commenter: "Commenter",
  viewer: "Viewer",
}

export const roleDescriptions: Record<Role, string> = {
  admin: "Everything, plus people, API keys, datasources and folder access.",
  editor: "Create and change dashboards and send work to the agent.",
  commenter: "View dashboards and comment on them.",
  viewer: "View dashboards only.",
}
