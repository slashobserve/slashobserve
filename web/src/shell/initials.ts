function words(value: string): string[] {
  return value
    .trim()
    .split(/[\s._-]+/)
    .filter(Boolean)
}

function fromWords(parts: string[]): string {
  if (parts.length >= 2) return parts[0][0] + parts[1][0]
  if (parts.length === 1) return parts[0].slice(0, 2)
  return ""
}

export function initials(name: string, email = ""): string {
  const fromName = fromWords(words(name))
  const result = fromName || fromWords(words(email.split("@")[0] ?? ""))
  return result ? result.toUpperCase() : "?"
}
