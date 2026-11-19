import { parseAnnotationTime } from "./time"

export type Scope = "dashboard" | "panel" | "global"

export type Fields = {
  text: string
  tags: string
  from: string
  to: string
  scope: Scope
}

export type Errors = Partial<Record<keyof Fields, string>>

export type CreateBody = {
  dashboard_id: string | null
  panel_id?: string
  time_from: string
  time_to?: string
  text: string
  tags: string[]
}

export type UpdateBody = { text: string; tags: string[] }

type Checked<T> = { ok: true; body: T } | { ok: false; errors: Errors }

const maxTextLength = 2000
const maxTags = 10
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/

function textError(text: string): string | null {
  const trimmed = text.trim()
  if (trimmed === "") return "Enter what happened."
  if ([...trimmed].length > maxTextLength) {
    return `Use at most ${maxTextLength} characters.`
  }
  return null
}

function parseTags(text: string): { tags: string[] } | { error: string } {
  const tags = [
    ...new Set(
      text
        .split(/[\s,]+/)
        .map((tag) => tag.toLowerCase())
        .filter((tag) => tag !== "")
    ),
  ]
  if (tags.length > maxTags) return { error: `Use at most ${maxTags} tags.` }
  const bad = tags.filter((tag) => !tagPattern.test(tag))
  if (bad.length > 0) {
    return {
      error: `Tags use letters, digits, dots, dashes and underscores, start with a letter or digit, and have at most 40 characters: ${bad.join(", ")}`,
    }
  }
  return { tags }
}

export function validateEdit(
  fields: Pick<Fields, "text" | "tags">
): Checked<UpdateBody> {
  const errors: Errors = {}
  const text = textError(fields.text)
  if (text !== null) errors.text = text
  const tags = parseTags(fields.tags)
  if ("error" in tags) errors.tags = tags.error
  if (errors.text !== undefined || "error" in tags) {
    return { ok: false, errors }
  }
  return { ok: true, body: { text: fields.text.trim(), tags: tags.tags } }
}

export function validateNew(
  fields: Fields,
  where: { dashboardId: string; panelId: string | undefined },
  zone: string | undefined,
  now: number
): Checked<CreateBody> {
  const errors: Errors = {}
  const text = textError(fields.text)
  if (text !== null) errors.text = text
  const tags = parseTags(fields.tags)
  if ("error" in tags) errors.tags = tags.error
  const from = parseAnnotationTime(fields.from, zone, now)
  if (from === null) {
    errors.from =
      "Enter a time such as 2026-11-18 14:22, or now-15m, from 1970 to 2099."
  }
  const to =
    fields.to.trim() === ""
      ? undefined
      : parseAnnotationTime(fields.to, zone, now)
  if (to === null) {
    errors.to =
      "Enter a time such as 2026-11-18 14:38, or leave it empty for one moment."
  } else if (from !== null && to !== undefined && to < from) {
    errors.to = "The end must not be before the start."
  }
  if (
    Object.keys(errors).length > 0 ||
    from === null ||
    to === null ||
    "error" in tags
  ) {
    return { ok: false, errors }
  }
  const body: CreateBody = {
    dashboard_id: fields.scope === "global" ? null : where.dashboardId,
    time_from: new Date(from).toISOString(),
    text: fields.text.trim(),
    tags: tags.tags,
  }
  if (fields.scope === "panel" && where.panelId !== undefined) {
    body.panel_id = where.panelId
  }
  if (to !== undefined) body.time_to = new Date(to).toISOString()
  return { ok: true, body }
}
