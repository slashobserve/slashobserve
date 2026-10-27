import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "traces",
  supported: true,
  load: () => import("./traces-panel"),
}
