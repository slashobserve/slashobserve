import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "logs",
  supported: true,
  minWidth: 300,
  minHeight: 320,
  load: () => import("./logs-panel"),
}
