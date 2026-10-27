import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "logs",
  supported: true,
  load: () => import("./logs-panel"),
}
