import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "canvas",
  supported: true,
  load: () => import("./canvas-panel"),
}
