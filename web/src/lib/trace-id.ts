const traceIdText = /^(?:[0-9a-f]{16}|[0-9a-f]{32})$/
const allZeros = /^0+$/

export const traceIdToken = "${__value.traceId}"

export function validTraceId(value: unknown): string | null {
  if (typeof value !== "string") return null
  const text = value.trim().toLowerCase()
  return traceIdText.test(text) && !allZeros.test(text) ? text : null
}
