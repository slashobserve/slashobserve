import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "status-history",
  supported: true,
  minWidth: 240,
  minHeight: 240,
  anchors: () => import("../state-timeline/anchors"),
  load: () => import("./status-history-panel"),
}
