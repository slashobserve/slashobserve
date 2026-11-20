import type { QueryClient } from "@tanstack/react-query"

const heldIds = new WeakMap<QueryClient, string[]>()

export function keepOrder<T extends { id: string }>(
  held: readonly string[],
  items: readonly T[]
): T[] {
  const rank = new Map(held.map((id, index) => [id, index]))
  const arrivals = items.filter((item) => !rank.has(item.id))
  const known = items
    .filter((item) => rank.has(item.id))
    .sort((a, b) => rank.get(a.id)! - rank.get(b.id)!)
  return [...arrivals, ...known]
}

export function holdOrder<T extends { id: string }>(
  client: QueryClient,
  items: readonly T[]
): T[] {
  const ordered = keepOrder(heldIds.get(client) ?? [], items)
  heldIds.set(
    client,
    ordered.map((item) => item.id)
  )
  return ordered
}
