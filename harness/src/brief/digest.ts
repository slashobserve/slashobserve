import { byteLength } from "./gofmt"

export type Discovery = {
  datasource: string
  of: string
  names: string[]
}

const maxDigestBytes = 4 << 10
const digestOpen =
  "<discovered>\nNames this run already found; use them without listing them again.\n"
const digestClosed = "</discovered>"

export function addDiscovery(
  entries: readonly Discovery[],
  x: Discovery
): Discovery[] {
  if (x.names.length === 0) {
    return [...entries]
  }
  return [
    ...entries.filter((e) => e.datasource !== x.datasource || e.of !== x.of),
    x,
  ]
}

export function digestText(entries: readonly Discovery[]): string {
  let out = digestOpen
  let written = 0
  for (const e of [...entries].reverse()) {
    const line =
      "- " + e.datasource + " " + e.of + ": " + e.names.join(", ") + "\n"
    if (
      byteLength(out) + byteLength(line) + byteLength(digestClosed) >
      maxDigestBytes
    ) {
      break
    }
    out += line
    written++
  }
  return written === 0 ? "" : out + digestClosed
}
