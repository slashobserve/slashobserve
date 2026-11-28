export function must<T>(value: T | undefined): T {
  if (value === undefined) throw new Error("expected a value")
  return value
}
