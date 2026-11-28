export function withoutKeys<T extends object, K extends keyof T>(
  value: T,
  keys: readonly K[]
): Omit<T, K> {
  const copy = { ...value }
  for (const key of keys) delete copy[key]
  return copy
}
