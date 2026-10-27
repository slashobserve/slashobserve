import type { Selections } from "@/api/keys"

import { ALL } from "../search"

const reference = /\$\{(\w+)\}|\$(\w+)/g

export function interpolateTitle(
  title: string,
  selections: Selections
): string {
  return title.replace(reference, (match, braced?: string, bare?: string) => {
    const values = selections[braced ?? bare ?? ""]
    if (!values || values.length === 0) return match
    return values.map((value) => (value === ALL ? "All" : value)).join(", ")
  })
}
