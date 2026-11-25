import { ApiError } from "@/api/errors"
import type { StatusTone } from "@/components/status-label"
import { loadProblemText } from "@/service-accounts/queries"

import type { ContextSource, ContextTool } from "./queries"

export type Audience = "everyone" | "admins"

export type SourceInput = {
  uid: string
  name: string
  description: string
  url: string
  audience: Audience
  secretHeader: string
  secret: string
  clearSecret: boolean
}

export type SourceProblems = Partial<Record<keyof SourceInput, string>>

const uidPattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/
const headerPattern = /^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,100}$/
const maxNameCharacters = 100
const maxDescriptionCharacters = 300
const maxURLBytes = 2048
const maxSecretBytes = 4096

export const audienceLabels: Record<Audience, string> = {
  everyone: "Everyone",
  admins: "Admins only",
}

function hasControlOrFormatCharacter(text: string): boolean {
  return hasControlCharacter(text) || /\p{Cf}/u.test(text)
}

function hasControlCharacter(text: string): boolean {
  return [...text].some((char) => {
    const code = char.charCodeAt(0)
    return code < 0x20 || code === 0x7f
  })
}

export function trimSource(input: SourceInput): SourceInput {
  return {
    ...input,
    uid: input.uid.trim(),
    name: input.name.trim(),
    description: input.description.trim(),
    url: input.url.trim(),
    secretHeader: input.secretHeader.trim(),
  }
}

function urlProblem(url: string): string | undefined {
  if (url === "") return "Enter the address of the MCP server."
  if (new TextEncoder().encode(url).length > maxURLBytes) {
    return `The address can be at most ${maxURLBytes} characters.`
  }
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return "Enter a full https address, such as https://mcp.example.com/mcp."
  }
  if (parsed.protocol !== "https:") {
    return "The address must start with https://."
  }
  if (parsed.username !== "" || parsed.password !== "") {
    return "The address cannot hold a user name or password. Use the secret field."
  }
  if (parsed.search !== "" || parsed.hash !== "" || url.endsWith("?")) {
    return "The address cannot hold a query or a # fragment. Use the secret field."
  }
  if (
    !parsed.hostname.includes(".") ||
    /^[\d.]+$/.test(parsed.hostname) ||
    parsed.hostname.startsWith("[")
  ) {
    return "The address must use a public domain name, like mcp.example.com. localhost and IP addresses are refused."
  }
  return undefined
}

export function sourceProblems(
  raw: SourceInput,
  mode: "add" | "edit",
  secretStored: boolean
): SourceProblems {
  const input = trimSource(raw)
  const problems: SourceProblems = {}
  if (mode === "add" && !uidPattern.test(input.uid)) {
    problems.uid =
      "Use up to 40 letters, digits, hyphens or underscores, starting with a letter or digit."
  }
  if ([...input.name].length > maxNameCharacters) {
    problems.name = `The name can be at most ${maxNameCharacters} characters.`
  }
  if ([...input.description].length > maxDescriptionCharacters) {
    problems.description = `The description can be at most ${maxDescriptionCharacters} characters.`
  } else if (hasControlOrFormatCharacter(input.description)) {
    problems.description = "The description must be on one line."
  }
  const url = urlProblem(input.url)
  if (url !== undefined) problems.url = url
  if (input.secret !== "") {
    if (new TextEncoder().encode(input.secret).length > maxSecretBytes) {
      problems.secret = `The secret can be at most ${maxSecretBytes} characters.`
    } else if (hasControlCharacter(input.secret)) {
      problems.secret =
        "The secret cannot hold line breaks or control characters."
    }
  }
  const sendsSecret =
    input.secret !== "" || (secretStored && !input.clearSecret)
  if (sendsSecret && !headerPattern.test(input.secretHeader)) {
    problems.secretHeader =
      "Enter the header that carries the secret, such as Authorization or X-Api-Key."
  }
  return problems
}

export function createBody(input: SourceInput) {
  const trimmed = trimSource(input)
  const body: {
    uid: string
    name?: string
    description?: string
    url: string
    audience: Audience
    secret_header?: string
    secret?: string
  } = { uid: trimmed.uid, url: trimmed.url, audience: trimmed.audience }
  if (trimmed.name !== "") body.name = trimmed.name
  if (trimmed.description !== "") body.description = trimmed.description
  if (trimmed.secret !== "") {
    body.secret = trimmed.secret
    body.secret_header = trimmed.secretHeader
  }
  return body
}

export function updateBody(initial: ContextSource, input: SourceInput) {
  const trimmed = trimSource(input)
  const body: {
    name?: string
    description?: string
    url?: string
    audience?: Audience
    secret_header?: string
    secret?: string
    clear_secret?: boolean
  } = {}
  if (trimmed.name !== "" && trimmed.name !== initial.name) {
    body.name = trimmed.name
  }
  if (trimmed.description !== (initial.description ?? "")) {
    body.description = trimmed.description
  }
  if (trimmed.url !== initial.url) body.url = trimmed.url
  if (trimmed.audience !== initial.audience) body.audience = trimmed.audience
  if (trimmed.clearSecret) {
    body.clear_secret = true
    return body
  }
  if (trimmed.secret !== "") body.secret = trimmed.secret
  if (trimmed.secretHeader !== (initial.secret_header ?? "")) {
    body.secret_header = trimmed.secretHeader
  }
  return body
}

export function isDomainNotAllowed(error: unknown): boolean {
  return (
    error instanceof ApiError && error.code === "outbound_domain_not_allowed"
  )
}

export function contextProblemText(error: unknown): string {
  if (!(error instanceof ApiError)) return loadProblemText(error)
  switch (error.code) {
    case "secret_key_missing":
      return "Set SLASHOBSERVE_SECRET_KEY on the server and restart to store a secret."
    case "too_many_context_sources":
      return "At most 50 context sources can exist. Delete one first."
    case "too_many_domains":
      return "The outbound allowlist is full. Remove a domain first."
    case "uid_taken":
      return "A context source with this id already exists."
    case "provisioned":
      return "This source comes from the context sources file on the server. Change the file instead."
    case "source_changed":
      return "The source changed while the server was being asked. Run discovery again."
    case "tool_changed":
      return "The server described this tool differently since you read it. Review it again."
    case "tool_refused":
      return "The server does not mark this tool read-only, so it cannot be approved."
    case "outbound_domain_not_allowed":
      return "The outbound allowlist is set by configuration and does not include this domain."
  }
  if (error.status === 404) return "That context source no longer exists."
  return error.message
}

const failureTexts: Record<string, string> = {
  outbound_domain_not_allowed: "The domain is not on the outbound allowlist",
  unauthorized: "The server refused the secret",
  timeout: "The server did not answer in time",
  not_an_mcp_server: "The address did not answer as an MCP server",
  unreachable: "The server could not be reached",
  too_many_tools: "The server lists more tools than the limit of 200",
  secret_key_missing: "SLASHOBSERVE_SECRET_KEY is not set on the server",
  secret_unreadable: "The secret cannot be read with the current key",
}

export function failureText(code: string | null): string {
  if (code === null) return "The last check failed"
  return failureTexts[code] ?? code
}

export type SourceStatus = { tone: StatusTone; label: string; detail: string }

export function sourceStatus(source: ContextSource): SourceStatus {
  switch (source.state) {
    case "unchecked":
      return {
        tone: "none",
        label: "Not checked",
        detail: "Discover the tools to check the server.",
      }
    case "unreachable":
      return {
        tone: "critical",
        label: "Unreachable",
        detail: failureText(source.failure),
      }
    case "ready":
      return { tone: "ok", label: "Ready", detail: toolSummary(source.tools) }
  }
}

export function toolSummary(counts: ContextSource["tools"]): string {
  const parts = [`${counts.approved} approved`]
  if (counts.offered > 0) parts.push(`${counts.offered} to review`)
  if (counts.changed > 0) parts.push(`${counts.changed} changed`)
  if (counts.gone > 0) parts.push(`${counts.gone} gone`)
  if (counts.refused > 0) parts.push(`${counts.refused} refused`)
  return parts.join(", ")
}

export type ToolStatus = { tone: StatusTone; label: string; detail: string }

export function toolStatus(tool: ContextTool): ToolStatus {
  switch (tool.state) {
    case "approved":
      return {
        tone: "ok",
        label: "Approved",
        detail: "The agent may call this tool as pinned.",
      }
    case "offered":
      return {
        tone: "none",
        label: "To review",
        detail: "The server marks it read-only. The agent cannot call it yet.",
      }
    case "changed":
      return {
        tone: "warning",
        label: "Changed",
        detail:
          "The server describes it differently from what you approved. It is not served until you approve it again.",
      }
    case "gone":
      return {
        tone: "warning",
        label: "Gone",
        detail:
          "The server no longer lists it. It returns when the server lists it unchanged.",
      }
    case "refused":
      return {
        tone: "critical",
        label: "Refused",
        detail: tool.refusal ?? "It cannot be approved.",
      }
  }
}

export function hasReadOnlyToApprove(tools: ContextTool[]): boolean {
  return tools.some((tool) => tool.state === "offered")
}
