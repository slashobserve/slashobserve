import type { components } from "@/api/schema.gen"

type Mention = components["schemas"]["Mention"]

type MentionRef = Pick<Mention, "kind" | "id">

export type MentionSegment =
  | { kind: "text"; text: string }
  | { kind: "mention"; ref: MentionRef; name: string; raw: string }

export type MentionPerson = { id: string; name: string }

export type MentionAgent = {
  kind: "observe" | "grant" | "service_account"
  id: string
  name: string
}

export type MentionOption = {
  value: string
  label: string
  token: string
  agent: boolean
  hint: string | null
}

export const observeAgent: MentionAgent = {
  kind: "observe",
  id: "",
  name: "Observe Agent",
}

const observeHandle = "observe"
const agentMention: MentionRef = { kind: "agent", id: observeHandle }

const uuid =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
const handleRune = "[\\p{L}\\p{Nd}_-]"
const mentionPattern = new RegExp(
  `@\\[([^\\]\\n]*)\\]\\((user|grant|service_account):(${uuid})\\)|(?<!${handleRune})@${observeHandle}(?!${handleRune})`,
  "gu"
)

export function mentionSegments(body: string): MentionSegment[] {
  const segments: MentionSegment[] = []
  let from = 0
  for (const match of body.matchAll(mentionPattern)) {
    if (match.index > from) {
      segments.push({ kind: "text", text: body.slice(from, match.index) })
    }
    const [raw, name, kind, id] = match
    segments.push(
      id === undefined
        ? { kind: "mention", ref: agentMention, name: observeHandle, raw }
        : {
            kind: "mention",
            ref: { kind: kind as MentionRef["kind"], id },
            name,
            raw,
          }
    )
    from = match.index + raw.length
  }
  if (from < body.length)
    segments.push({ kind: "text", text: body.slice(from) })
  return segments
}

function tokenName(name: string, fallback: string): string {
  const cleaned = name.replace(/[[\]\n\r]/g, "").trim()
  return cleaned === "" ? fallback : cleaned
}

export function mentionToken(person: MentionPerson): string {
  return `@[${tokenName(person.name, "user")}](user:${person.id})`
}

export function agentToken(agent: MentionAgent): string {
  if (agent.kind === "observe") return `@${observeHandle}`
  return `@[${tokenName(agent.name, "agent")}](${agent.kind}:${agent.id})`
}

const agentHints: Record<MentionAgent["kind"], string> = {
  observe: `@${observeHandle}`,
  grant: "Your agent",
  service_account: "Service account",
}

function agentOption(agent: MentionAgent): MentionOption {
  return {
    value:
      agent.kind === "observe"
        ? `agent:${observeHandle}`
        : `${agent.kind}:${agent.id}`,
    label: agent.name,
    token: agentToken(agent),
    agent: true,
    hint: agentHints[agent.kind],
  }
}

export function mentionedAgents(body: string): { key: string; name: string }[] {
  const found = new Map<string, string>()
  for (const segment of mentionSegments(body)) {
    if (segment.kind !== "mention" || segment.ref.kind === "user") continue
    const key = `${segment.ref.kind}:${segment.ref.id.toLowerCase()}`
    if (found.has(key)) continue
    found.set(
      key,
      segment.ref.kind === "agent" ? observeAgent.name : segment.name
    )
  }
  return [...found].map(([key, name]) => ({ key, name }))
}

export function agentMentionsIn(body: string): number {
  return mentionedAgents(body).length
}

export function mentionOptions(
  people: readonly MentionPerson[],
  agents: readonly MentionAgent[]
): MentionOption[] {
  const offered = new Map<string, MentionOption>()
  for (const agent of [observeAgent, ...agents]) {
    const option = agentOption(agent)
    offered.set(option.value, option)
  }
  return [
    ...offered.values(),
    ...people.map((person) => ({
      value: `user:${person.id}`,
      label: person.name === "" ? "Unnamed" : person.name,
      token: mentionToken(person),
      agent: false,
      hint: null,
    })),
  ]
}

export function mentionTriggerAt(value: string, caret: number): number | null {
  if (caret < 1 || value[caret - 1] !== "@") return null
  const before = value[caret - 2]
  return before === undefined || /\s/.test(before) ? caret - 1 : null
}

export function insertMention(
  value: string,
  start: number,
  end: number,
  token: string
): { value: string; caret: number } {
  const before = value.slice(0, start)
  const after = value.slice(end)
  const lead = before === "" || /\s$/.test(before) ? "" : " "
  const trail = /^\s/.test(after) ? "" : " "
  const inserted = `${lead}${token}${trail}`
  return {
    value: `${before}${inserted}${after}`,
    caret: before.length + inserted.length,
  }
}

export function threadParticipants(
  authors: readonly { kind: string; id: string; name: string }[]
): MentionPerson[] {
  const people = new Map<string, MentionPerson>()
  for (const author of authors) {
    if (author.kind !== "user" || people.has(author.id)) continue
    people.set(author.id, { id: author.id, name: author.name })
  }
  return [...people.values()]
}

export function mentionName(
  segment: Extract<MentionSegment, { kind: "mention" }>,
  recorded: readonly Mention[]
): string | null {
  const found = recorded.find(
    (mention) =>
      mention.kind === segment.ref.kind && mention.id === segment.ref.id
  )
  return found === undefined ? null : found.name
}
