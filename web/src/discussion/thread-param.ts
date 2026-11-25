const threadIdPattern = /^[A-Za-z0-9_-]{1,64}$/

export function threadParam(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  return typeof item === "string" && threadIdPattern.test(item)
    ? item
    : undefined
}
