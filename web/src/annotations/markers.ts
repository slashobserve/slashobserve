import type { Annotation } from "./queries"

export function annotationsFor(
  list: readonly Annotation[],
  panelId: string,
  tags: readonly string[] | undefined
): Annotation[] {
  return list.filter(
    (a) =>
      (a.panel_id === null || a.panel_id === "" || a.panel_id === panelId) &&
      (tags === undefined ||
        tags.length === 0 ||
        a.tags.some((tag) => tags.includes(tag)))
  )
}

export type MarkerSpan = {
  annotation: Annotation
  x: number
  x2: number | null
}

export function markerSpans(
  list: readonly Annotation[],
  { fromMs, toMs, width }: { fromMs: number; toMs: number; width: number }
): MarkerSpan[] {
  if (!(width > 0) || !(toMs > fromMs)) return []
  const span = toMs - fromMs
  const place = (ms: number) =>
    ((Math.min(Math.max(ms, fromMs), toMs) - fromMs) / span) * width
  const out: { span: MarkerSpan; start: number }[] = []
  for (const annotation of list) {
    const start = Date.parse(annotation.time_from)
    if (Number.isNaN(start)) continue
    const end =
      annotation.time_to === null ? null : Date.parse(annotation.time_to)
    if (end !== null && Number.isNaN(end)) continue
    if (end === null) {
      if (start < fromMs || start > toMs) continue
    } else if (end < fromMs || start > toMs) {
      continue
    }
    out.push({
      span: {
        annotation,
        x: place(start),
        x2: end === null ? null : place(end),
      },
      start,
    })
  }
  out.sort((a, b) => a.span.x - b.span.x || a.start - b.start)
  return out.map((item) => item.span)
}

export type MarkerFlag = { x: number; annotations: Annotation[] }

function newestFirst(a: Annotation, b: Annotation): number {
  return Date.parse(b.time_from) - Date.parse(a.time_from)
}

export function clusterFlags(
  spans: readonly MarkerSpan[],
  gapPx = 8
): MarkerFlag[] {
  const flags: MarkerFlag[] = []
  let current: MarkerFlag | undefined
  for (const span of spans) {
    if (current === undefined || span.x - current.x > gapPx) {
      current = { x: span.x, annotations: [] }
      flags.push(current)
    }
    current.annotations.push(span.annotation)
  }
  for (const flag of flags) flag.annotations.sort(newestFirst)
  return flags
}

export type TextPart = { text: string; href?: string }

const trailingPunctuation = /[.,;:!?)]+$/

function linkTarget(token: string, origin: string): string | undefined {
  if (token.startsWith("/")) {
    if (token.startsWith("//")) return undefined
    try {
      const url = new URL(token, origin)
      return url.origin === new URL(origin).origin ? token : undefined
    } catch {
      return undefined
    }
  }
  try {
    const url = new URL(token)
    return url.protocol === "http:" || url.protocol === "https:"
      ? token
      : undefined
  } catch {
    return undefined
  }
}

export function linkParts(text: string, origin: string): TextPart[] {
  const parts: TextPart[] = []
  const push = (part: TextPart) => {
    const last = parts.at(-1)
    if (part.href === undefined && last && last.href === undefined) {
      last.text += part.text
    } else {
      parts.push(part)
    }
  }
  for (const token of text.split(/(\s+)/)) {
    if (token === "") continue
    const trail = trailingPunctuation.exec(token)?.[0] ?? ""
    const core = token.slice(0, token.length - trail.length)
    const href = core === "" ? undefined : linkTarget(core, origin)
    if (href === undefined) {
      push({ text: token })
      continue
    }
    push({ text: core, href })
    if (trail !== "") push({ text: trail })
  }
  return parts
}

export function cardLeft(
  flagX: number,
  cardWidth: number,
  areaWidth: number
): number {
  const centred = flagX - cardWidth / 2
  return Math.min(Math.max(centred, 0), Math.max(0, areaWidth - cardWidth))
}
