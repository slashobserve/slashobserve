import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "logs",
  supported: true,
  minWidth: 300,
  load: () => import("./logs-panel"),
}
