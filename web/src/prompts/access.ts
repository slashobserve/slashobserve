// "object &" keeps TypeScript's weak-type check from rejecting a Principal that has no role field yet.
export function canDriveAgent(principal: object & { role?: string }): boolean {
  return (
    principal.role === undefined ||
    principal.role === "admin" ||
    principal.role === "editor"
  )
}
