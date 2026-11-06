import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "nodeGraph",
  supported: true,
  minHeight: 320,
  load: () => import("./node-graph-panel"),
}
