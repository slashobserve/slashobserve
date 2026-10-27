export function toggleOrdered(
  order: readonly string[],
  current: readonly string[],
  value: string,
  on: boolean
): string[] {
  const chosen = new Set(current)
  if (on) chosen.add(value)
  else chosen.delete(value)
  return [
    ...order.filter((item) => chosen.has(item)),
    ...current.filter((item) => !order.includes(item) && chosen.has(item)),
  ]
}
