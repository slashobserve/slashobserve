import { goJSON } from "../brief/gofmt"

export type Node =
  | { kind: "object"; keys: string[]; vals: Node[] }
  | { kind: "array"; vals: Node[] }
  | { kind: "string"; text: string }
  | { kind: "number"; text: string }
  | { kind: "literal"; text: string }

const number = /-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/y
const stringToken =
  /"(?:[^"\\\u0000-\u001f]|\\(?:["\\/bfnrt]|u[0-9a-fA-F]{4}))*"/y
const whitespace = /[ \t\n\r]*/y

class Parser {
  at = 0
  constructor(readonly text: string) {}

  space(): void {
    whitespace.lastIndex = this.at
    whitespace.exec(this.text)
    this.at = whitespace.lastIndex
  }

  token(re: RegExp): string | undefined {
    re.lastIndex = this.at
    const m = re.exec(this.text)
    if (!m) {
      return undefined
    }
    this.at = re.lastIndex
    return m[0]
  }

  value(depth: number): Node | undefined {
    if (depth > 10000) {
      return undefined
    }
    this.space()
    const c = this.text[this.at]
    if (c === "{") {
      this.at++
      const node: Node = { kind: "object", keys: [], vals: [] }
      this.space()
      if (this.text[this.at] === "}") {
        this.at++
        return node
      }
      for (;;) {
        this.space()
        const key = this.token(stringToken)
        if (key === undefined) {
          return undefined
        }
        this.space()
        if (this.text[this.at] !== ":") {
          return undefined
        }
        this.at++
        const child = this.value(depth + 1)
        if (!child) {
          return undefined
        }
        node.keys.push(JSON.parse(key) as string)
        node.vals.push(child)
        this.space()
        const next = this.text[this.at++]
        if (next === "}") {
          return node
        }
        if (next !== ",") {
          return undefined
        }
      }
    }
    if (c === "[") {
      this.at++
      const node: Node = { kind: "array", vals: [] }
      this.space()
      if (this.text[this.at] === "]") {
        this.at++
        return node
      }
      for (;;) {
        const child = this.value(depth + 1)
        if (!child) {
          return undefined
        }
        node.vals.push(child)
        this.space()
        const next = this.text[this.at++]
        if (next === "]") {
          return node
        }
        if (next !== ",") {
          return undefined
        }
      }
    }
    if (c === '"') {
      const raw = this.token(stringToken)
      return raw === undefined
        ? undefined
        : { kind: "string", text: JSON.parse(raw) as string }
    }
    for (const literal of ["true", "false", "null"]) {
      if (this.text.startsWith(literal, this.at)) {
        this.at += literal.length
        return { kind: "literal", text: literal }
      }
    }
    const raw = this.token(number)
    return raw === undefined ? undefined : { kind: "number", text: raw }
  }
}

export function parseNode(text: string): Node | undefined {
  const p = new Parser(text)
  const node = p.value(0)
  if (!node) {
    return undefined
  }
  p.space()
  return p.at === text.length ? node : undefined
}

export function parseFirst(text: string): Node | undefined {
  return new Parser(text).value(0)
}

export function get(n: Node | undefined, key: string): Node | undefined {
  if (n?.kind !== "object") {
    return undefined
  }
  const i = n.keys.indexOf(key)
  return i < 0 ? undefined : n.vals[i]
}

export function str(n: Node | undefined, key: string): string {
  const v = get(n, key)
  return v?.kind === "string" ? v.text : ""
}

export function set(n: Node & { kind: "object" }, key: string, v: Node): void {
  const i = n.keys.indexOf(key)
  if (i >= 0) {
    n.vals[i] = v
    return
  }
  n.keys.push(key)
  n.vals.push(v)
}

export function encode(n: Node): string {
  switch (n.kind) {
    case "object":
      return (
        "{" +
        n.keys
          .map((key, i) => goJSON(key) + ":" + encode(n.vals[i] as Node))
          .join(",") +
        "}"
      )
    case "array":
      return "[" + n.vals.map(encode).join(",") + "]"
    case "string":
      return goJSON(n.text)
    default:
      return n.text
  }
}
