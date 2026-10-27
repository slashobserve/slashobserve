import type { StatusTone } from "@/components/status-label"
import type { components } from "@/api/schema.gen"

type ImportCounts = components["schemas"]["ImportCounts"]

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? "" : "s"}`
}

export function countsText(counts: ImportCounts): string {
  const parts = [
    counts.imported > 0 ? `${counts.imported} imported` : null,
    counts.partial > 0 ? `${counts.partial} partial` : null,
    counts.placeholder > 0 ? `${counts.placeholder} placeholder` : null,
    counts.unverified > 0 ? `${counts.unverified} unverified` : null,
  ].filter((part) => part !== null)
  const head = `${plural(counts.total, "panel")} at import`
  return parts.length === 0 ? head : `${head}: ${parts.join(", ")}`
}

export function importStatus(counts: ImportCounts): {
  tone: StatusTone
  text: string
} {
  if (counts.unverified > 0) {
    return { tone: "warning", text: `${counts.unverified} unverified` }
  }
  if (counts.placeholder > 0) {
    return { tone: "none", text: plural(counts.placeholder, "placeholder") }
  }
  return { tone: "ok", text: "Imported" }
}

export function fixPrompt(title: string, panelId: string): string {
  return title === ""
    ? `Fix the unverified panel ${panelId}.`
    : `Fix the unverified panel "${title}" (${panelId}).`
}
