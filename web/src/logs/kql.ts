export type RangeOp = ">" | ">=" | "<" | "<="

export type KqlNode =
  | { kind: "and" | "or"; items: KqlNode[] }
  | { kind: "not"; item: KqlNode }
  | {
      kind: "match"
      field: string
      value: string
      quoted: boolean
      wildcard: boolean
    }
  | {
      kind: "range"
      field: string
      op: RangeOp
      value: string
      quoted: boolean
    }
  | { kind: "exists"; field: string }
  | { kind: "free"; value: string; quoted: boolean; wildcard: boolean }
  | { kind: "group"; field?: string; item: KqlNode }
  | { kind: "in"; field: string; values: string[]; op: "or" | "and" }

export type KqlError = { message: string; start: number; end: number }

type Parsed = { node: KqlNode; start: number; end: number }

type Segment = { text: string; star: boolean }

type TokenKind =
  | "eof"
  | "word"
  | "quoted"
  | "lparen"
  | "rparen"
  | "colon"
  | "rangeop"
  | "and"
  | "or"
  | "not"
  | "lbrace"

type Token = {
  kind: TokenKind
  text: string
  segments: Segment[]
  start: number
  end: number
}

const maxNodes = 200
const maxDepth = 20
const tooComplex = "query too complex"
const punctuation: Record<string, TokenKind> = {
  "(": "lparen",
  ")": "rparen",
  ":": "colon",
  "{": "lbrace",
}
const keywords: Record<string, TokenKind> = {
  and: "and",
  or: "or",
  not: "not",
}
const wordBreakers = '():<>"{'
const bareWordBreakers = '():<>"{\\'

class KqlSyntaxError extends Error {
  readonly start: number
  readonly end: number

  constructor(message: string, start: number, end: number) {
    super(message)
    this.start = start
    this.end = end
  }
}

function isSpace(c: string): boolean {
  return c === " " || c === "\t" || c === "\n" || c === "\r"
}

function isWordChar(c: string): boolean {
  return !isSpace(c) && !wordBreakers.includes(c)
}

function lexQuoted(text: string, start: number): Token {
  let value = ""
  for (let i = start + 1; i < text.length; i++) {
    const c = text[i]
    if (c === "\\") {
      if (i + 1 < text.length) {
        i++
        value += text[i]
      }
    } else if (c === '"') {
      return { kind: "quoted", text: value, segments: [], start, end: i + 1 }
    } else {
      value += c
    }
  }
  throw new KqlSyntaxError("unterminated string", start, text.length)
}

function lexWord(text: string, start: number): Token {
  const segments: Segment[] = []
  let literal = ""
  let escaped = false
  let pos = start
  function flush() {
    if (literal === "") return
    segments.push({ text: literal, star: false })
    literal = ""
  }
  while (pos < text.length && isWordChar(text[pos])) {
    const c = text[pos]
    if (c === "\\" && pos + 1 < text.length) {
      const next = text[pos + 1]
      if (isSpace(next)) {
        throw new KqlSyntaxError(
          "quote a value that contains spaces",
          pos,
          pos + 2
        )
      }
      literal += next
      escaped = true
      pos += 2
    } else if (c === "*") {
      flush()
      segments.push({ text: "", star: true })
      pos++
    } else {
      literal += c
      pos++
    }
  }
  flush()
  const value = segments.map((s) => (s.star ? "*" : s.text)).join("")
  const keyword = keywords[value.toLowerCase()]
  const isKeyword =
    keyword !== undefined &&
    Object.hasOwn(keywords, value.toLowerCase()) &&
    !escaped &&
    segments.length === 1
  return {
    kind: isKeyword ? keyword : "word",
    text: value,
    segments,
    start,
    end: pos,
  }
}

function lex(text: string): Token[] {
  const tokens: Token[] = []
  let pos = 0
  for (;;) {
    while (pos < text.length && isSpace(text[pos])) pos++
    if (pos >= text.length) {
      tokens.push({ kind: "eof", text: "", segments: [], start: pos, end: pos })
      return tokens
    }
    const c = text[pos]
    const start = pos
    let token: Token
    if (Object.hasOwn(punctuation, c)) {
      token = {
        kind: punctuation[c],
        text: c,
        segments: [],
        start,
        end: start + 1,
      }
    } else if (c === "<" || c === ">") {
      const end = text[start + 1] === "=" ? start + 2 : start + 1
      token = {
        kind: "rangeop",
        text: text.slice(start, end),
        segments: [],
        start,
        end,
      }
    } else if (c === '"') {
      token = lexQuoted(text, start)
    } else {
      token = lexWord(text, start)
    }
    tokens.push(token)
    pos = token.end
  }
}

function quoteToken(text: string): string {
  return JSON.stringify(text)
}

function startsPrimary(token: Token): boolean {
  return (
    token.kind === "word" ||
    token.kind === "quoted" ||
    token.kind === "lparen" ||
    token.kind === "not" ||
    token.kind === "lbrace"
  )
}

class Parser {
  private pos = 0
  private nodes = 0
  private depth = 0

  private readonly text: string
  private readonly tokens: Token[]

  constructor(text: string, tokens: Token[]) {
    this.text = text
    this.tokens = tokens
  }

  parse(): Parsed {
    const root = this.or()
    if (this.peek().kind !== "eof") throw this.unexpected()
    return root
  }

  private peek(ahead = 0): Token {
    const index = Math.min(this.pos + ahead, this.tokens.length - 1)
    return this.tokens[index]
  }

  private advance(): Token {
    const token = this.peek()
    if (this.pos < this.tokens.length - 1) this.pos++
    return token
  }

  private or(): Parsed {
    return this.list("or", "or", () => this.and())
  }

  private and(): Parsed {
    return this.list("and", "and", () => this.not())
  }

  private list(
    kind: "and" | "or",
    separator: TokenKind,
    item: () => Parsed
  ): Parsed {
    const first = item()
    const children = [first]
    for (;;) {
      if (this.peek().kind === separator) {
        this.advance()
      } else if (!(kind === "and" && startsPrimary(this.peek()))) {
        if (children.length === 1) return first
        const last = children[children.length - 1]
        return this.node({
          node: { kind, items: children.map((child) => child.node) },
          start: first.start,
          end: last.end,
        })
      }
      children.push(item())
    }
  }

  private not(): Parsed {
    if (this.peek().kind !== "not") return this.primary()
    return this.negation(() => this.not())
  }

  private negation(operand: () => Parsed): Parsed {
    const token = this.advance()
    if (!startsPrimary(this.peek())) {
      throw new KqlSyntaxError(
        "expected a value after NOT",
        token.start,
        token.end
      )
    }
    const child = this.nested(token, operand)
    return this.node({
      node: { kind: "not", item: child.node },
      start: token.start,
      end: child.end,
    })
  }

  private primary(): Parsed {
    const token = this.peek()
    switch (token.kind) {
      case "lparen": {
        const { child, end } = this.parens(() => this.or())
        return this.node({
          node: { kind: "group", item: child.node },
          start: token.start,
          end,
        })
      }
      case "word": {
        const next = this.peek(1).kind
        if (next === "colon" || next === "rangeop") return this.fieldQuery()
        return this.literal("free", this.advance(), null)
      }
      case "quoted":
        return this.literal("free", this.advance(), null)
      case "lbrace":
        throw this.nestedQuery(token)
    }
    throw this.unexpected()
  }

  private fieldQuery(): Parsed {
    const field = this.advance()
    const op = this.advance()
    if (field.text.startsWith("*")) {
      throw new KqlSyntaxError(
        "a field name cannot start with *",
        field.start,
        field.end
      )
    }
    if (field.text === "kql" && op.kind === "colon") {
      throw new KqlSyntaxError(
        "kql: is a form prefix, not a query",
        field.start,
        op.end
      )
    }
    if (op.kind === "rangeop") return this.rangeQuery(field, op)
    const value = this.peek()
    switch (value.kind) {
      case "lparen": {
        const { child, end } = this.parens(() => this.valueOr(field))
        return this.node({
          node: { kind: "group", field: field.text, item: child.node },
          start: field.start,
          end,
        })
      }
      case "word":
      case "quoted": {
        const literal = this.literal("match", this.advance(), field)
        return { ...literal, start: field.start }
      }
      case "lbrace":
        throw this.nestedQuery(value)
    }
    throw new KqlSyntaxError('expected a value after ":"', op.start, op.end)
  }

  private rangeQuery(field: Token, op: Token): Parsed {
    const value = this.peek()
    if (value.kind !== "word" && value.kind !== "quoted") {
      throw new KqlSyntaxError(
        "expected a value after " + quoteToken(op.text),
        op.start,
        op.end
      )
    }
    this.advance()
    return this.node({
      node: {
        kind: "range",
        field: field.text,
        op: op.text as RangeOp,
        value: value.text,
        quoted: value.kind === "quoted",
      },
      start: field.start,
      end: value.end,
    })
  }

  private valueOr(field: Token): Parsed {
    return this.list("or", "or", () => this.valueAnd(field))
  }

  private valueAnd(field: Token): Parsed {
    return this.list("and", "and", () => this.valueNot(field))
  }

  private valueNot(field: Token): Parsed {
    const token = this.peek()
    switch (token.kind) {
      case "not":
        return this.negation(() => this.valueNot(field))
      case "lparen": {
        const { child, end } = this.parens(() => this.valueOr(field))
        return this.node({
          node: { kind: "group", item: child.node },
          start: token.start,
          end,
        })
      }
      case "word":
      case "quoted":
        return this.literal("match", this.advance(), field)
      case "lbrace":
        throw this.nestedQuery(token)
    }
    throw this.unexpected()
  }

  private literal(
    kind: "match" | "free",
    value: Token,
    field: Token | null
  ): Parsed {
    const quoted = value.kind === "quoted"
    const wildcard = !quoted && value.segments.some((s) => s.star)
    const onlyStar =
      !quoted && value.segments.length === 1 && value.segments[0].star
    const span = { start: value.start, end: value.end }
    if (kind === "match" && field !== null) {
      if (onlyStar) {
        return this.node({
          node: { kind: "exists", field: field.text },
          ...span,
        })
      }
      return this.node({
        node: {
          kind: "match",
          field: field.text,
          value: value.text,
          quoted,
          wildcard,
        },
        ...span,
      })
    }
    return this.node({
      node: { kind: "free", value: value.text, quoted, wildcard },
      ...span,
    })
  }

  private parens(inner: () => Parsed): { child: Parsed; end: number } {
    const open = this.peek()
    const child = this.nested(open, () => {
      this.advance()
      return inner()
    })
    const closer = this.peek()
    if (closer.kind !== "rparen") {
      const at = closer.kind === "eof" ? open : closer
      throw new KqlSyntaxError('expected ")"', at.start, at.end)
    }
    this.advance()
    return { child, end: closer.end }
  }

  private nested(at: Token, inner: () => Parsed): Parsed {
    this.depth++
    try {
      if (this.depth > maxDepth) {
        throw new KqlSyntaxError(tooComplex, at.start, at.end)
      }
      return inner()
    } finally {
      this.depth--
    }
  }

  private node(parsed: Parsed): Parsed {
    this.nodes++
    if (this.nodes > maxNodes) {
      throw new KqlSyntaxError(tooComplex, parsed.start, parsed.end)
    }
    return parsed
  }

  private unexpected(): KqlSyntaxError {
    const token = this.peek()
    if (token.kind === "eof") {
      const previous = this.tokens[this.pos - 1]
      return new KqlSyntaxError(
        "expected a value after " + quoteToken(previous.text),
        previous.start,
        previous.end
      )
    }
    return new KqlSyntaxError(
      "unexpected " + quoteToken(token.text),
      token.start,
      token.end
    )
  }

  private nestedQuery(open: Token): KqlSyntaxError {
    let depth = 0
    for (let i = open.start; i < this.text.length; i++) {
      if (this.text[i] === "{") depth++
      else if (this.text[i] === "}" && --depth === 0) {
        return new KqlSyntaxError(
          "nested queries are not supported",
          open.start,
          i + 1
        )
      }
    }
    return new KqlSyntaxError(
      "nested queries are not supported",
      open.start,
      this.text.length
    )
  }
}

export function parseKql(text: string): {
  node: KqlNode | null
  error: KqlError | null
} {
  try {
    const tokens = lex(text)
    if (tokens[0].kind === "eof") return { node: null, error: null }
    return { node: new Parser(text, tokens).parse().node, error: null }
  } catch (thrown) {
    if (thrown instanceof KqlSyntaxError) {
      const { message, start, end } = thrown
      return { node: null, error: { message, start, end } }
    }
    throw thrown
  }
}

export function escapeKql(value: string): string {
  return '"' + value.replace(/[\\"]/g, "\\$&") + '"'
}

function isKeyword(value: string): boolean {
  return Object.hasOwn(keywords, value.toLowerCase())
}

export function needsQuotes(value: string): boolean {
  if (value === "" || isKeyword(value)) return true
  for (const c of value) {
    if (isSpace(c) || bareWordBreakers.includes(c)) return true
  }
  return false
}

export function valueNode(value: string): {
  value: string
  quoted: boolean
  wildcard: boolean
} {
  const quoted = needsQuotes(value)
  return { value, quoted, wildcard: !quoted && value.includes("*") }
}

function bareWord(value: string, wildcard: boolean): string {
  let out = ""
  for (const c of value) {
    if (c === "*") out += wildcard ? "*" : "\\*"
    else if (bareWordBreakers.includes(c)) out += "\\" + c
    else out += c
  }
  return isKeyword(value) ? "\\" + out : out
}

function valueText(value: string, quoted: boolean, wildcard: boolean): string {
  return quoted ? escapeKql(value) : bareWord(value, wildcard)
}

function printNode(node: KqlNode, inField: boolean): string {
  switch (node.kind) {
    case "and":
    case "or":
      return node.items
        .map((item) => printNode(item, inField))
        .join(` ${node.kind} `)
    case "not":
      return "not " + printNode(node.item, inField)
    case "match": {
      const value = valueText(node.value, node.quoted, node.wildcard)
      return inField ? value : `${node.field} : ${value}`
    }
    case "range":
      return `${node.field} ${node.op} ${valueText(node.value, node.quoted || needsQuotes(node.value), false)}`
    case "exists":
      return inField ? "*" : `${node.field} : *`
    case "free":
      return valueText(node.value, node.quoted, node.wildcard)
    case "group":
      return node.field === undefined
        ? `(${printNode(node.item, inField)})`
        : `${node.field} : (${printNode(node.item, true)})`
    case "in": {
      const values = node.values.map((value) => {
        const { quoted, wildcard } = valueNode(value)
        return valueText(value, quoted, wildcard)
      })
      return `${node.field} : (${values.join(` ${node.op} `)})`
    }
  }
}

export function kqlText(node: KqlNode): string {
  return printNode(node, false)
}

function appendClause(text: string, clause: string): string {
  const trimmed = text.trim()
  if (trimmed === "") return clause
  const { node, error } = parseKql(trimmed)
  if (error !== null || node?.kind === "or") {
    return `(${trimmed}) and ${clause}`
  }
  return `${trimmed} and ${clause}`
}

function negated(clause: string, negate: boolean): string {
  return negate ? "not " + clause : clause
}

export function appendKql(
  text: string,
  field: string,
  value: string,
  negate: boolean
): string {
  return appendClause(text, negated(`${field}:${escapeKql(value)}`, negate))
}

export function appendExists(
  text: string,
  field: string,
  negate: boolean
): string {
  return appendClause(text, negated(`${field}:*`, negate))
}

export function appendIn(
  text: string,
  field: string,
  values: string[],
  negate: boolean
): string {
  if (values.length === 0) return text
  const list = values.map(escapeKql).join(" or ")
  return appendClause(text, negated(`${field}:(${list})`, negate))
}

export function appendRange(
  text: string,
  field: string,
  op: RangeOp,
  value: string
): string {
  const word = valueText(value, needsQuotes(value), false)
  return appendClause(text, `${field} ${op} ${word}`)
}

function collectFields(node: KqlNode, into: Set<string>) {
  switch (node.kind) {
    case "and":
    case "or":
      node.items.forEach((item) => collectFields(item, into))
      return
    case "not":
      collectFields(node.item, into)
      return
    case "group":
      if (node.field !== undefined) into.add(node.field)
      collectFields(node.item, into)
      return
    case "match":
    case "range":
    case "exists":
    case "in":
      into.add(node.field)
      return
    case "free":
      return
  }
}

export function kqlFields(text: string): string[] {
  const { node } = parseKql(text)
  if (node === null) return []
  const fields = new Set<string>()
  collectFields(node, fields)
  return [...fields]
}

function asValueList(node: KqlNode): KqlNode | null {
  if (node.kind !== "group" || node.field === undefined) return node
  const inner = node.item
  const matches =
    inner.kind === "and" || inner.kind === "or" ? inner.items : [inner]
  const values: string[] = []
  for (const match of matches) {
    if (match.kind !== "match" || match.wildcard) return null
    values.push(match.value)
  }
  const op = inner.kind === "and" ? "and" : "or"
  return { kind: "in", field: node.field, values, op }
}

function isLeaf(node: KqlNode): boolean {
  return (
    node.kind === "match" ||
    node.kind === "range" ||
    node.kind === "exists" ||
    node.kind === "in" ||
    node.kind === "free"
  )
}

function flatClause(node: KqlNode): KqlNode | null {
  const item = asValueList(node)
  if (item === null) return null
  if (isLeaf(item)) return item
  if (item.kind !== "not") return null
  const inner = asValueList(item.item)
  return inner !== null && isLeaf(inner) ? { kind: "not", item: inner } : null
}

export function flatClauses(node: KqlNode): KqlNode[] | null {
  const items = node.kind === "and" ? node.items : [node]
  const clauses: KqlNode[] = []
  for (const item of items) {
    const clause = flatClause(item)
    if (clause === null) return null
    clauses.push(clause)
  }
  return clauses
}
