import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "status-history",
  supported: true,
  load: () => import("./status-history-panel"),
}
