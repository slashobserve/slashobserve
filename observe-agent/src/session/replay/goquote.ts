const printable = /^[\p{L}\p{M}\p{N}\p{P}\p{S}]$/u

const shortEscapes: Record<number, string> = {
  0x07: "\\a",
  0x08: "\\b",
  0x0c: "\\f",
  0x0a: "\\n",
  0x0d: "\\r",
  0x09: "\\t",
  0x0b: "\\v",
}

function hex(value: number, width: number): string {
  return value.toString(16).padStart(width, "0")
}

function quoteRune(rune: string): string {
  const code = rune.codePointAt(0) ?? 0
  if (rune === '"' || rune === "\\") {
    return "\\" + rune
  }
  if (code === 0x20 || printable.test(rune)) {
    return rune
  }
  const short = shortEscapes[code]
  if (short !== undefined) {
    return short
  }
  if (code < 0x20 || code === 0x7f) {
    return "\\x" + hex(code, 2)
  }
  return code < 0x10000 ? "\\u" + hex(code, 4) : "\\U" + hex(code, 8)
}

export function goQuote(text: string): string {
  let quoted = '"'
  for (const rune of text) {
    quoted += quoteRune(rune)
  }
  return quoted + '"'
}
