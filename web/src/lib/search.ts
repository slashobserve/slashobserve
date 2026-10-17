export type SearchValue = string | string[]

export function parseSearch(searchStr: string): Record<string, SearchValue> {
  const params = new URLSearchParams(
    searchStr.startsWith("?") ? searchStr.slice(1) : searchStr
  )
  const out: Record<string, SearchValue> = {}
  for (const [key, value] of params) {
    const prev = out[key]
    if (prev === undefined) out[key] = value
    else out[key] = Array.isArray(prev) ? [...prev, value] : [prev, value]
  }
  return out
}

function isPrimitive(value: unknown): value is string | number | boolean {
  return (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  )
}

function appendValue(params: URLSearchParams, key: string, value: unknown) {
  if (isPrimitive(value)) {
    params.append(key, String(value))
    return
  }
  if (import.meta.env.DEV) {
    throw new Error(`Search param "${key}" is not a string, number or boolean`)
  }
}

export function stringifySearch(search: Record<string, unknown>): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(search)) {
    if (value === undefined || value === null) continue
    if (Array.isArray(value)) {
      for (const item of value) appendValue(params, key, item)
    } else {
      appendValue(params, key, value)
    }
  }
  const text = params.toString()
  return text ? `?${text}` : ""
}
