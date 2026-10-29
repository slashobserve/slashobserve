import type { Principal } from "@/lib/auth"
import { roleAtLeast } from "@/lib/roles"

export function canDriveAgent(principal: Principal): boolean {
  return roleAtLeast(principal.role, "editor")
}
