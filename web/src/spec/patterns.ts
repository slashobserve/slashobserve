const slashed = /^\/(.*?)\/(g?i?m?y?s?)$/

export function compileAnchoredPattern(pattern: string): RegExp | null {
  try {
    return new RegExp(`^(?:${pattern})$`)
  } catch {
    return null
  }
}

export function compileNamePattern(pattern: string): RegExp | null {
  if (!pattern.startsWith("/")) return compileAnchoredPattern(pattern)
  const match = slashed.exec(pattern)
  if (!match) return null
  const [, source = "", flags = ""] = match
  try {
    return new RegExp(source, flags.replace(/[gy]/g, ""))
  } catch {
    return null
  }
}
