import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "flamegraph",
  supported: true,
  load: () => import("./flamegraph-panel"),
}
