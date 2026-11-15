import type { components } from "@/api/schema.gen"
import { initials } from "@/shell/initials"

export type Actor = components["schemas"]["Actor"]

type AuthorMarkKind = "person" | "observe-agent" | "outside-agent"

export type PersonTint = 1 | 2 | 3 | 4 | 5 | 6

export type AuthorMarkModel = {
  kind: AuthorMarkKind
  text: string
  tint: PersonTint | null
}

export function personTint(id: string): PersonTint {
  let sum = 0
  for (let i = 0; i < id.length; i++) sum += id.charCodeAt(i)
  return ((sum % 6) + 1) as PersonTint
}

export const observeAgentMark: AuthorMarkModel = {
  kind: "observe-agent",
  text: "/",
  tint: null,
}

export function markOfAuthor(
  author: Pick<Actor, "kind" | "id" | "name">
): AuthorMarkModel {
  if (author.kind === "service_account") {
    return { kind: "outside-agent", text: initials(author.name), tint: null }
  }
  return {
    kind: "person",
    text: initials(author.name),
    tint: personTint(author.id),
  }
}

export function markOf(
  actor: Pick<Actor, "kind" | "id" | "name" | "origin" | "client" | "run_id">
): AuthorMarkModel {
  if (actor.run_id !== null) return observeAgentMark
  if (actor.origin === "mcp") {
    return {
      kind: "outside-agent",
      text: initials(actor.client ?? "MCP"),
      tint: null,
    }
  }
  return markOfAuthor(actor)
}
