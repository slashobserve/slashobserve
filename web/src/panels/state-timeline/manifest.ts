import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "state-timeline",
  supported: true,
  minWidth: 240,
  minHeight: 240,
  load: () => import("./state-timeline-panel"),
}
