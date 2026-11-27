import { goQuote } from "./goquote"

export type JsonNode =
  | { kind: "null"; raw: string }
  | { kind: "bool"; raw: string }
  | { kind: "number"; raw: string }
  | { kind: "string"; raw: string; value: string }
  | { kind: "array"; raw: string; items: JsonNode[] }
  | {
      kind: "object"
      raw: string
      members: Array<{ key: string; value: JsonNode }>
    }

export class JsonSyntaxError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "JsonSyntaxError"
  }
}

const whitespace = new Set([" ", "\t", "\r", "\n"])
const escapes: Record<string, string> = {
  '"': '"',
  "\\": "\\",
  "/": "/",
  b: "\b",
  f: "\f",
  n: "\n",
  r: "\r",
  t: "\t",
}
const literals = { t: "true", f: "false", n: "null" } as const

function quoteChar(char: string): string {
  if (char === "'") {
    return "'\\''"
  }
  if (char === '"') {
    return "'\"'"
  }
  return "'" + goQuote(char).slice(1, -1) + "'"
}

function isDigit(char: string | undefined): boolean {
  return char !== undefined && char >= "0" && char <= "9"
}

class Scanner {
  private pos = 0
  private readonly text: string

  constructor(text: string) {
    this.text = text
  }

  get rest(): string {
    return this.text.slice(this.pos)
  }

  private invalid(char: string, context: string): never {
    throw new JsonSyntaxError(`invalid character ${quoteChar(char)} ${context}`)
  }

  private truncated(): never {
    throw new JsonSyntaxError("unexpected EOF")
  }

  private skipSpace(): void {
    while (whitespace.has(this.text[this.pos] ?? "")) {
      this.pos++
    }
  }

  private peekChar(): string | undefined {
    const code = this.text.codePointAt(this.pos)
    return code === undefined ? undefined : String.fromCodePoint(code)
  }

  document(): JsonNode {
    this.skipSpace()
    if (this.pos >= this.text.length) {
      throw new JsonSyntaxError("EOF")
    }
    return this.value()
  }

  private next(): string {
    this.skipSpace()
    const char = this.peekChar()
    if (char === undefined) {
      this.truncated()
    }
    return char
  }

  private value(): JsonNode {
    const char = this.next()
    const start = this.pos
    switch (char) {
      case "{":
        return this.object(start)
      case "[":
        return this.array(start)
      case '"': {
        const value = this.string()
        return { kind: "string", raw: this.text.slice(start, this.pos), value }
      }
      case "t":
      case "f":
      case "n":
        return this.literal(char, start)
      default:
        if (char === "-" || isDigit(char)) {
          return this.number(start)
        }
        return this.invalid(char, "looking for beginning of value")
    }
  }

  private literal(first: "t" | "f" | "n", start: number): JsonNode {
    const word = literals[first]
    for (let i = 1; i < word.length; i++) {
      const char = this.text[this.pos + i]
      if (char === undefined) {
        this.truncated()
      }
      if (char !== word[i]) {
        this.invalid(
          char,
          `in literal ${word} (expecting ${quoteChar(word[i] ?? "")})`
        )
      }
    }
    this.pos += word.length
    const raw = this.text.slice(start, this.pos)
    return first === "n" ? { kind: "null", raw } : { kind: "bool", raw }
  }

  private number(start: number): JsonNode {
    const digits = (context: string): void => {
      const char = this.text[this.pos]
      if (char === undefined) {
        this.truncated()
      }
      if (!isDigit(char)) {
        this.invalid(char, context)
      }
      while (isDigit(this.text[this.pos])) {
        this.pos++
      }
    }
    if (this.text[this.pos] === "-") {
      this.pos++
      const char = this.text[this.pos]
      if (char === undefined) {
        this.truncated()
      }
      if (!isDigit(char)) {
        this.invalid(char, "in numeric literal")
      }
    }
    if (this.text[this.pos] === "0") {
      this.pos++
    } else {
      digits("in numeric literal")
    }
    if (this.text[this.pos] === ".") {
      this.pos++
      digits("in numeric literal")
    }
    if (this.text[this.pos] === "e" || this.text[this.pos] === "E") {
      this.pos++
      if (this.text[this.pos] === "+" || this.text[this.pos] === "-") {
        this.pos++
      }
      digits("in numeric literal")
    }
    return { kind: "number", raw: this.text.slice(start, this.pos) }
  }

  private string(): string {
    this.pos++
    let value = ""
    for (;;) {
      const char = this.peekChar()
      if (char === undefined) {
        this.truncated()
      }
      this.pos += char.length
      if (char === '"') {
        return value
      }
      if (char < " ") {
        this.invalid(char, "in string")
      }
      if (char !== "\\") {
        value += char
        continue
      }
      value += this.escape()
    }
  }

  private escape(): string {
    const char = this.text[this.pos]
    if (char === undefined) {
      this.truncated()
    }
    const plain = escapes[char]
    if (plain !== undefined) {
      this.pos++
      return plain
    }
    if (char !== "u") {
      const rune = this.peekChar() ?? char
      return rune.length === 1 && rune < "\u0080"
        ? this.invalidEscape("\\" + rune)
        : this.invalidMultibyteEscape(rune)
    }
    const first = this.hex4()
    if (first < 0xd800 || first > 0xdfff) {
      return String.fromCharCode(first)
    }
    const low = this.pairedLowSurrogate(first)
    return low === undefined
      ? String.fromCharCode(0xfffd)
      : String.fromCharCode(first, low)
  }

  private invalidMultibyteEscape(rune: string): never {
    const lead = Buffer.from(rune)[0] ?? 0
    throw new JsonSyntaxError(
      `invalid escape sequence "\\\\\\x${lead.toString(16)}" in string`
    )
  }

  private invalidEscape(what: string): never {
    const plain = /^[^`\p{Z}\p{C}\ufffd\t-\r]*$/u.test(what)
    const shown = plain ? "`" + what + "`" : goQuote(what)
    throw new JsonSyntaxError(`invalid escape sequence ${shown} in string`)
  }

  private pairedLowSurrogate(high: number): number | undefined {
    if (high > 0xdbff || this.text.slice(this.pos, this.pos + 2) !== "\\u") {
      return undefined
    }
    const digits = this.text.slice(this.pos + 2, this.pos + 6)
    if (!/^[0-9a-fA-F]{4}$/.test(digits)) {
      return undefined
    }
    const low = parseInt(digits, 16)
    if (low < 0xdc00 || low > 0xdfff) {
      return undefined
    }
    this.pos += 6
    return low
  }

  private hex4(): number {
    const sequence = this.text.slice(this.pos - 1, this.pos + 5)
    const digits = sequence.slice(2)
    if (digits.length < 4) {
      if (/^[0-9a-fA-F]*$/.test(digits)) {
        this.truncated()
      }
      this.invalidEscape(sequence)
    }
    if (!/^[0-9a-fA-F]{4}$/.test(digits)) {
      this.invalidEscape(sequence)
    }
    this.pos += 5
    return parseInt(digits, 16)
  }

  private array(start: number): JsonNode {
    this.pos++
    const items: JsonNode[] = []
    let char = this.next()
    if (char === "]") {
      this.pos++
      return { kind: "array", raw: this.text.slice(start, this.pos), items }
    }
    for (;;) {
      items.push(this.value())
      char = this.next()
      this.pos += char.length
      if (char === "]") {
        return { kind: "array", raw: this.text.slice(start, this.pos), items }
      }
      if (char !== ",") {
        this.invalid(char, "after array element")
      }
    }
  }

  private object(start: number): JsonNode {
    this.pos++
    const members: Array<{ key: string; value: JsonNode }> = []
    let char = this.next()
    if (char === "}") {
      this.pos++
      return { kind: "object", raw: this.text.slice(start, this.pos), members }
    }
    for (;;) {
      char = this.next()
      if (char !== '"') {
        this.invalid(char, "looking for beginning of object key string")
      }
      const key = this.string()
      char = this.next()
      this.pos += char.length
      if (char !== ":") {
        this.invalid(char, "after object key")
      }
      members.push({ key, value: this.value() })
      char = this.next()
      this.pos += char.length
      if (char === "}") {
        return {
          kind: "object",
          raw: this.text.slice(start, this.pos),
          members,
        }
      }
      if (char !== ",") {
        this.invalid(char, "after object key:value pair")
      }
    }
  }
}

export type JsonDocument = { node: JsonNode; trailing: boolean }

export function parseJsonDocument(text: string): JsonDocument {
  const scanner = new Scanner(text)
  const node = scanner.document()
  return { node, trailing: /[^ \t\r\n]/.test(scanner.rest) }
}
