const maxSlugLength = 60
const historySegment = "versions"

export function slugOf(title: string): string {
  const slug = title
    .replace(/[A-Z]/g, (letter) => letter.toLowerCase())
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, maxSlugLength)
    .replace(/-+$/, "")
  return slug === historySegment ? "" : slug
}

function slugParam(title: string | undefined): string | undefined {
  return title === undefined ? undefined : slugOf(title) || undefined
}

export function dashboardLink(dashboardId: string, title?: string) {
  return {
    to: "/d/$dashboardId/{-$slug}",
    params: { dashboardId, slug: slugParam(title) },
  } as const
}

function path(...segments: string[]): string {
  return "/" + segments.map(encodeURIComponent).join("/")
}

export function dashboardPath(dashboardId: string, title?: string): string {
  const slug = slugParam(title)
  return slug === undefined
    ? path("d", dashboardId)
    : path("d", dashboardId, slug)
}

export function folderPath(folderId: string): string {
  return path("f", folderId)
}

export function chatPath(requestId: string): string {
  return path("c", requestId)
}

export function buildPath(requestId: string): string {
  return path("b", requestId)
}
