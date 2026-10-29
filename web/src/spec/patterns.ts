const slashed = /^\/(.*?)\/(g?i?m?y?s?)$/

export function compileNamePattern(pattern: string): RegExp | null {
  try {
    if (!pattern.startsWith("/")) return new RegExp(`^(?:${pattern})$`)
    const match = slashed.exec(pattern)
    if (!match) return null
    return new RegExp(match[1], match[2].replace(/[gy]/g, ""))
  } catch {
    return null
  }
}
