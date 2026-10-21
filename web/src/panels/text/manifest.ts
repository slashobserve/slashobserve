import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "text",
  supported: false,
  load: () => import("./text-panel"),
}
