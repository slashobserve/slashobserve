import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "state-timeline",
  supported: true,
  load: () => import("./state-timeline-panel"),
}
