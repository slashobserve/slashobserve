import { firstString } from "@/lib/search"

const threadIdPattern = /^[A-Za-z0-9_-]{1,64}$/

export function threadParam(value: unknown): string | undefined {
  const item = firstString(value)
  return item !== undefined && threadIdPattern.test(item) ? item : undefined
}
