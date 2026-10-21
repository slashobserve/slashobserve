import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "logs",
  supported: false,
  load: () => import("./logs-panel"),
}
