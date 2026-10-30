import type { Dashboard } from "@/spec"

export type Details = { title: string; description: string; tags: string[] }

const maxTitleLength = 200
const maxDescriptionLength = 2000
const maxTags = 20
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/

function length(text: string): number {
  return [...text].length
}

export function detailsOf(spec: Dashboard): Details {
  return {
    title: spec.title,
    description: spec.description ?? "",
    tags: spec.tags ?? [],
  }
}

export function parseTags(
  text: string
): { tags: string[] } | { error: string } {
  const tags = [
    ...new Set(
      text
        .split(/[\s,]+/)
        .map((tag) => tag.trim().toLowerCase())
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

export function titleError(title: string): string | null {
  const trimmed = title.trim()
  if (trimmed === "") return "Enter a title."
  if (length(trimmed) > maxTitleLength) {
    return `Use at most ${maxTitleLength} characters.`
  }
  return null
}

export function descriptionError(text: string): string | null {
  if (length(text) > maxDescriptionLength) {
    return `Use at most ${maxDescriptionLength} characters.`
  }
  return null
}

export function applyDetails(spec: Dashboard, details: Details): Dashboard {
  const next: Dashboard = { ...spec, title: details.title.trim() }
  const description = details.description.trim()
  if (description === "") delete next.description
  else next.description = description
  if (details.tags.length === 0) delete next.tags
  else next.tags = details.tags
  return next
}

export function sameDetails(a: Details, b: Details): boolean {
  return (
    a.title === b.title &&
    a.description === b.description &&
    a.tags.length === b.tags.length &&
    a.tags.every((tag, index) => tag === b.tags[index])
  )
}

export function detailsSummary(before: Details, after: Details): string {
  const onlyTitle = sameDetails({ ...after, title: before.title }, before)
  return onlyTitle && before.title !== after.title
    ? `Rename to "${after.title}"`
    : "Edit dashboard details"
}

export function detailsBlockReason(state: {
  pinned: boolean
  editingLayout: boolean
  editingPanel: boolean
}): string | null {
  if (state.pinned) return "open the latest version first"
  if (state.editingLayout) return "finish the layout first"
  if (state.editingPanel) return "close the panel editor first"
  return null
}
