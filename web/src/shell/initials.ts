function words(value: string): string[] {
  return value
    .trim()
    .split(/[\s._-]+/)
    .filter(Boolean)
}

function fromWords(parts: string[]): string {
  const [first, second] = parts
  if (first === undefined) return ""
  if (second !== undefined) return first.charAt(0) + second.charAt(0)
  return first.slice(0, 2)
}

export function initials(name: string, email = ""): string {
  const fromName = fromWords(words(name))
  const result = fromName || fromWords(words(email.split("@")[0] ?? ""))
  return result ? result.toUpperCase() : "?"
}
