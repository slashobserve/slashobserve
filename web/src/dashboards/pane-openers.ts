export const shareButtonId = "dashboard-share-button"
export const activityButtonId = "dashboard-activity-button"

export type DockPane = "share" | "activity"

export function paneOpener(pane: DockPane): HTMLElement | null {
  return document.getElementById(
    pane === "share" ? shareButtonId : activityButtonId
  )
}
