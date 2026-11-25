import { roleSourceHint } from "./role-source"
import type { User } from "./queries"

export function RoleSourceLine({
  person,
}: {
  person: Pick<User, "role_source" | "role_group" | "role_set_by">
}) {
  const hint = roleSourceHint(person)
  return (
    <span
      className="block max-w-full truncate text-xs text-muted-foreground"
      title={hint}
    >
      {hint}
    </span>
  )
}
