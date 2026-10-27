import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "nodeGraph",
  supported: true,
  load: () => import("./node-graph-panel"),
}
