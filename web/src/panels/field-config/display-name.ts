import type { PreparedFrame } from "@/panels/transforms"

const token = /\$\{([^}]*)\}/g
const labelPrefix = "__field.labels."

function allLabels(labels: Record<string, string>): string {
  return Object.keys(labels)
    .map((key) => `${key}="${labels[key]}"`)
    .join(", ")
}

export function interpolateDisplayName(
  template: string,
  frame: PreparedFrame,
  fieldName: string,
  seriesName: string
): string {
  const labels = frame.labels ?? {}
  return template.replace(token, (written, inner: string) => {
    const name = inner.trim()
    if (name === "__series.name") return seriesName
    if (name === "__field.name") return fieldName
    if (name === "__field.labels") return allLabels(labels)
    if (name.startsWith(labelPrefix)) {
      const key = name.slice(labelPrefix.length)
      return Object.hasOwn(labels, key) ? labels[key] : written
    }
    return written
  })
}
