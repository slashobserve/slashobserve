import type { RendererManifest } from "../types"

export const manifest: RendererManifest = {
  type: "flamegraph",
  supported: true,
  minHeight: 320,
  load: () => import("./flamegraph-panel"),
}
