import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "text",
  supported: true,
  load: () => import("./text-panel"),
}
