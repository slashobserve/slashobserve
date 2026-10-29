import type { components } from "@/api/schema.gen"

type Role = components["schemas"]["Role"]

// "object &" keeps TypeScript's weak-type check from rejecting a Principal whose type has no role field yet.
export function canDriveAgent(principal: object & { role?: Role }): boolean {
  return principal.role === "admin" || principal.role === "editor"
}
