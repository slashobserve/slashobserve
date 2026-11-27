import { createHash } from "node:crypto"

type Cursor = { text: string; at: number }

function skipSpace(c: Cursor): void {
  while (c.at < c.text.length && " \t\n\r".includes(c.text[c.at] as string)) {
    c.at++
  }
}

function stringEnd(c: Cursor): number {
  let i = c.at + 1
  while (c.text[i] !== '"') {
    i += c.text[i] === "\\" ? 2 : 1
  }
  return i + 1
}

function value(c: Cursor): string {
  skipSpace(c)
  const first = c.text[c.at]
  if (first === "{") {
    c.at++
    const members = new Map<string, string>()
    skipSpace(c)
    while (c.text[c.at] !== "}") {
      skipSpace(c)
      const end = stringEnd(c)
      const name = JSON.parse(c.text.slice(c.at, end)) as string
      c.at = end
      skipSpace(c)
      c.at++
      members.set(name, value(c))
      skipSpace(c)
      if (c.text[c.at] === ",") {
        c.at++
      }
    }
    c.at++
    const names = [...members.keys()].sort()
    return `{${names.map((name) => `${JSON.stringify(name)}:${members.get(name)}`).join(",")}}`
  }
  if (first === "[") {
    c.at++
    const items: string[] = []
    skipSpace(c)
    while (c.text[c.at] !== "]") {
      items.push(value(c))
      skipSpace(c)
      if (c.text[c.at] === ",") {
        c.at++
      }
    }
    c.at++
    return `[${items.join(",")}]`
  }
  if (first === '"') {
    const end = stringEnd(c)
    const decoded = JSON.parse(c.text.slice(c.at, end)) as string
    c.at = end
    return JSON.stringify(decoded)
  }
  const start = c.at
  while (
    c.at < c.text.length &&
    !",]} \t\n\r".includes(c.text[c.at] as string)
  ) {
    c.at++
  }
  return c.text.slice(start, c.at)
}

export function canonicalJSON(text: string): string | undefined {
  try {
    JSON.parse(text)
  } catch {
    return undefined
  }
  return value({ text, at: 0 })
}

export function callKey(name: string, input: string): string {
  const body = canonicalJSON(input) ?? input
  return `${name}:${createHash("sha256").update(body).digest("hex")}`
}
