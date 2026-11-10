import type { components } from "@/api/schema.gen"

type Mention = components["schemas"]["Mention"]

type MentionRef = Pick<Mention, "kind" | "id">

export type MentionSegment =
  | { kind: "text"; text: string }
  | { kind: "mention"; ref: MentionRef; name: string; raw: string }

export type MentionPerson = { id: string; name: string }

const agentMention: MentionRef = { kind: "agent", id: "observe" }

const uuid =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
const handleRune = "[\\p{L}\\p{Nd}_-]"
const mentionPattern = new RegExp(
  `@\\[([^\\]\\n]*)\\]\\(user:(${uuid})\\)|(?<!${handleRune})@observe(?!${handleRune})`,
  "gu"
)

export function mentionSegments(body: string): MentionSegment[] {
  const segments: MentionSegment[] = []
  let from = 0
  for (const match of body.matchAll(mentionPattern)) {
    if (match.index > from) {
      segments.push({ kind: "text", text: body.slice(from, match.index) })
    }
    const [raw, name, id] = match
    segments.push(
      id === undefined
        ? { kind: "mention", ref: agentMention, name: "observe", raw }
        : { kind: "mention", ref: { kind: "user", id }, name, raw }
    )
    from = match.index + raw.length
  }
  if (from < body.length)
    segments.push({ kind: "text", text: body.slice(from) })
  return segments
}

export function mentionToken(person: MentionPerson): string {
  const name = person.name.replace(/[[\]\n\r]/g, "").trim()
  return `@[${name === "" ? "user" : name}](user:${person.id})`
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
  messages: readonly { author: { kind: string; id: string; name: string } }[]
): MentionPerson[] {
  const people = new Map<string, MentionPerson>()
  for (const { author } of messages) {
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
