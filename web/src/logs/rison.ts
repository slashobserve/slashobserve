export class RisonError extends Error {
  readonly offset: number

  constructor(message: string, offset: number) {
    super(`${message} at offset ${offset}`)
    this.name = "RisonError"
    this.offset = offset
  }
}

const maxDepth = 32
const notIdCharacters = " '!:(),*@$"
const numberPattern = /-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/y
const literals: Record<string, boolean | null> = { t: true, f: false, n: null }

class Reader {
  private at = 0
  private readonly text: string

  constructor(text: string) {
    this.text = text
  }

  read(): unknown {
    const value = this.value(0)
    if (this.at < this.text.length) {
      throw new RisonError("unexpected text after the value", this.at)
    }
    return value
  }

  private value(depth: number): unknown {
    if (depth > maxDepth) throw new RisonError("nested too deeply", this.at)
    const character = this.text[this.at]
    if (character === undefined) {
      throw new RisonError("unexpected end", this.at)
    }
    if (character === "!") return this.bang(depth)
    if (character === "(") return this.object(depth)
    if (character === "'") return this.string()
    if (character === "-" || (character >= "0" && character <= "9")) {
      return this.number()
    }
    return this.identifier()
  }

  private bang(depth: number): unknown {
    const marker = this.text[this.at + 1]
    if (marker === "(") {
      this.at += 2
      return this.list(() => this.value(depth + 1))
    }
    if (marker !== undefined && marker in literals) {
      this.at += 2
      return literals[marker]
    }
    throw new RisonError("unknown ! form", this.at)
  }

  private object(depth: number): Record<string, unknown> {
    this.at++
    const result: Record<string, unknown> = {}
    this.list(() => {
      const key = this.text[this.at] === "'" ? this.string() : this.identifier()
      if (this.text[this.at] !== ":") {
        throw new RisonError("expected :", this.at)
      }
      this.at++
      result[key] = this.value(depth + 1)
    })
    return result
  }

  private list(item: () => unknown): unknown[] {
    const items: unknown[] = []
    if (this.text[this.at] === ")") {
      this.at++
      return items
    }
    for (;;) {
      items.push(item())
      const next = this.text[this.at]
      this.at++
      if (next === ")") return items
      if (next !== ",") throw new RisonError("expected , or )", this.at - 1)
    }
  }

  private string(): string {
    const start = this.at
    this.at++
    let result = ""
    while (this.at < this.text.length) {
      const character = this.text[this.at]
      if (character === "'") {
        this.at++
        return result
      }
      if (character === "!") {
        const escaped = this.text[this.at + 1]
        if (escaped !== "'" && escaped !== "!") {
          throw new RisonError("unknown string escape", this.at)
        }
        result += escaped
        this.at += 2
      } else {
        result += character
        this.at++
      }
    }
    throw new RisonError("unterminated string", start)
  }

  private number(): number {
    numberPattern.lastIndex = this.at
    const match = numberPattern.exec(this.text)
    if (match === null) throw new RisonError("invalid number", this.at)
    this.at += match[0].length
    return Number(match[0])
  }

  private identifier(): string {
    const start = this.at
    while (
      this.at < this.text.length &&
      !notIdCharacters.includes(this.text[this.at])
    ) {
      this.at++
    }
    if (this.at === start) throw new RisonError("expected a value", start)
    return this.text.slice(start, this.at)
  }
}

export function decodeRison(text: string): unknown {
  return new Reader(text).read()
}
