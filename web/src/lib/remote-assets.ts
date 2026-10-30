const imageRoute = "/api/v1/assets/image?url="
const geojsonRoute = "/api/v1/assets/geojson?url="

function resolve(raw: string, origin: string): URL | null {
  if (raw.trim() === "") return null
  try {
    return new URL(raw, origin)
  } catch {
    return null
  }
}

function isRemoteHttps(url: URL, origin: string): boolean {
  return url.protocol === "https:" && url.origin !== origin
}

function sameOriginPath(url: URL, origin: string): string | null {
  const sameOrigin =
    (url.protocol === "https:" || url.protocol === "http:") &&
    url.origin === origin
  return sameOrigin ? url.pathname + url.search + url.hash : null
}

export function assetImageUrl(
  raw: string,
  origin = location.origin
): string | null {
  const url = resolve(raw, origin)
  if (!url) return null
  if (isRemoteHttps(url, origin))
    return imageRoute + encodeURIComponent(url.href)
  return sameOriginPath(url, origin)
}

export function assetGeojsonUrl(
  raw: string,
  origin = location.origin
): string | null {
  const url = resolve(raw, origin)
  if (!url || !isRemoteHttps(url, origin)) return null
  return geojsonRoute + encodeURIComponent(url.href)
}

export function remoteHost(
  raw: string,
  origin = location.origin
): string | null {
  const url = resolve(raw, origin)
  return url && isRemoteHttps(url, origin) ? url.hostname.toLowerCase() : null
}

export function hostAllowed(
  patterns: readonly string[],
  host: string
): boolean {
  const name = host.toLowerCase()
  return patterns.some((pattern) => {
    const p = pattern.toLowerCase()
    if (!p.startsWith("*.")) return p === name
    return name.endsWith(p.slice(1))
  })
}
