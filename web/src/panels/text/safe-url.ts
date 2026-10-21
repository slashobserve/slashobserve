const linkProtocols = new Set(["http:", "https:", "mailto:"])
const webProtocols = new Set(["http:", "https:"])

function resolve(url: string, origin: string): URL | null {
  try {
    return new URL(url, origin)
  } catch {
    return null
  }
}

export function safeUrl(url: string, origin = location.origin): string {
  const resolved = resolve(url, origin)
  return resolved && linkProtocols.has(resolved.protocol) ? url : ""
}

export function isExternal(url: string, origin = location.origin): boolean {
  const resolved = resolve(url, origin)
  if (!resolved) return true
  return webProtocols.has(resolved.protocol) && resolved.origin !== origin
}

export function isImageSource(url: string, origin = location.origin): boolean {
  if (url === "") return false
  const resolved = resolve(url, origin)
  if (!resolved) return false
  return (
    resolved.protocol === "https:" ||
    (webProtocols.has(resolved.protocol) && resolved.origin === origin)
  )
}
