export type DashboardEvent =
  | { type: "ready"; dashboardId: string; latestVersion: number }
  | { type: "resync"; dashboardId: string }
  | {
      type: "version.created"
      dashboardId: string
      version: number
      summary: string
      authorName: string
    }
  | { type: "dashboard.deleted"; dashboardId: string; authorName: string }
  | {
      type: "comment.created" | "comment.updated" | "revision.updated"
      dashboardId: string
    }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function text(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

function versionNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0
    ? value
    : undefined
}

function authorNameOf(raw: Record<string, unknown>): string {
  const author = isRecord(raw.author) ? raw.author : {}
  return typeof author.name === "string" ? author.name : ""
}

export function parseEvent(
  eventName: string,
  data: string
): DashboardEvent | null {
  let raw: unknown
  try {
    raw = JSON.parse(data)
  } catch {
    return null
  }
  if (!isRecord(raw)) return null
  const dashboardId = text(raw.dashboard_id)
  if (dashboardId === undefined) return null

  switch (eventName) {
    case "ready": {
      const latestVersion = versionNumber(raw.latest_version)
      if (latestVersion === undefined) return null
      return { type: "ready", dashboardId, latestVersion }
    }
    case "resync":
      return { type: "resync", dashboardId }
    case "dashboard.deleted":
      return {
        type: "dashboard.deleted",
        dashboardId,
        authorName: authorNameOf(raw),
      }
    case "version.created": {
      const version = versionNumber(raw.version)
      if (version === undefined) return null
      return {
        type: "version.created",
        dashboardId,
        version,
        summary: typeof raw.summary === "string" ? raw.summary : "",
        authorName: authorNameOf(raw),
      }
    }
    case "comment.created":
    case "comment.updated":
    case "revision.updated":
      return { type: eventName, dashboardId }
    default:
      return null
  }
}
