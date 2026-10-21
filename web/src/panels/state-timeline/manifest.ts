import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "state-timeline",
  supported: false,
  load: () => import("./state-timeline-panel"),
}
