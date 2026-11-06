import type { paths } from "@/api/schema.gen"

export type Star =
  paths["/stars"]["get"]["responses"][200]["content"]["application/json"]["items"][number]

export type StarTarget = Omit<Star, "starred_at">

export function withStar(
  items: readonly Star[],
  target: StarTarget,
  starred: boolean,
  now: Date
): Star[] {
  const rest = items.filter((item) => item.id !== target.id)
  if (!starred) return rest
  const existing = items.find((item) => item.id === target.id)
  if (existing !== undefined) return [...items]
  return [{ ...target, starred_at: now.toISOString() }, ...rest]
}
