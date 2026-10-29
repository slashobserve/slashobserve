import type { User } from "./queries"

export function roleSourceHint(
  person: Pick<User, "role_source" | "role_group" | "role_set_by">
): string {
  switch (person.role_source) {
    case "admin_emails":
      return "From SLASHOBSERVE_ADMIN_EMAILS"
    case "first_user":
      return "First person to sign in"
    case "group":
      return person.role_group === null
        ? "From an identity provider group"
        : `From the ${person.role_group} group`
    case "manual":
      return person.role_set_by === null
        ? "Set by an Admin"
        : `Set by ${person.role_set_by}`
    case "default":
      return "Default for new people"
  }
}
