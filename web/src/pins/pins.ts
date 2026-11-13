import type { paths } from "@/api/schema.gen"

export type Pin =
  paths["/stars"]["get"]["responses"][200]["content"]["application/json"]["items"][number]

export type PinTarget = Omit<Pin, "starred_at">

export function withPin(
  items: readonly Pin[],
  target: PinTarget,
  pinned: boolean,
  now: Date
): Pin[] {
  const rest = items.filter((item) => item.id !== target.id)
  if (!pinned) return rest
  const existing = items.find((item) => item.id === target.id)
  if (existing !== undefined) return [...items]
  return [{ ...target, starred_at: now.toISOString() }, ...rest]
}
