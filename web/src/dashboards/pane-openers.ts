export const shareButtonId = "dashboard-share-button"
export const activityButtonId = "dashboard-activity-button"
export const controlsButtonId = "dashboard-controls-button"

export type DockPane = "share" | "activity"

export function paneOpener(pane: DockPane): HTMLElement | null {
  return (
    document.getElementById(
      pane === "share" ? shareButtonId : activityButtonId
    ) ?? document.getElementById(controlsButtonId)
  )
}
