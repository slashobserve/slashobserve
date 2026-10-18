export type DashboardEvent =
  | { type: "ready"; dashboardId: string; latestVersion: number }
  | { type: "resync"; dashboardId: string }
  | {
      type: "version.created"
      dashboardId: string
      version: number
      restoredFrom: number | null
      summary: string
      authorName: string
    }
  | {
      type: "comment.created" | "comment.updated"
      dashboardId: string
      commentId: string
      panelId: string
      status: string
    }
  | {
      type: "revision.updated"
      dashboardId: string
      revisionId: string
      status: string
      version?: number
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
    case "version.created": {
      const version = versionNumber(raw.version)
      if (version === undefined) return null
      const author = isRecord(raw.author) ? raw.author : {}
      return {
        type: "version.created",
        dashboardId,
        version,
        restoredFrom: versionNumber(raw.restored_from) ?? null,
        summary: typeof raw.summary === "string" ? raw.summary : "",
        authorName: typeof author.name === "string" ? author.name : "",
      }
    }
    case "comment.created":
    case "comment.updated": {
      const commentId = text(raw.comment_id)
      const panelId = text(raw.panel_id)
      const status = text(raw.status)
      if (!commentId || !panelId || !status) return null
      return { type: eventName, dashboardId, commentId, panelId, status }
    }
    case "revision.updated": {
      const revisionId = text(raw.revision_id)
      const status = text(raw.status)
      if (!revisionId || !status) return null
      const version = versionNumber(raw.version)
      return {
        type: "revision.updated",
        dashboardId,
        revisionId,
        status,
        ...(version === undefined ? {} : { version }),
      }
    }
    default:
      return null
  }
}
