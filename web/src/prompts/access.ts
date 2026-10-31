import type { Principal } from "@/lib/auth"
import { roleAtLeast } from "@/lib/roles"

export function canSavePrompts(principal: Principal): boolean {
  return roleAtLeast(principal.role, "editor")
}
