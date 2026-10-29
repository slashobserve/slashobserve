import { validateBack } from "@/dashboards/search"

export type HistorySearch = {
  from?: number | undefined
  to?: number | undefined
  back?: string | undefined
}

const versionNumber = /^(0|[1-9][0-9]{0,8})$/

function versionParam(value: unknown): number | undefined {
  const item = Array.isArray(value) ? value[0] : value
  if (typeof item !== "string" && typeof item !== "number") return undefined
  const text = String(item)
  return versionNumber.test(text) ? Number(text) : undefined
}

export function validateHistorySearch(
  raw: Record<string, unknown>
): HistorySearch {
  return {
    from: versionParam(raw.from),
    to: versionParam(raw.to),
    back: validateBack(raw.back),
  }
}
