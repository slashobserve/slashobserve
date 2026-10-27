import { lazy, type LazyExoticComponent } from "react"

import type { DataPanelType } from "@/spec"

import { guardLoad } from "./renderer-load"
import type {
  PanelRenderer,
  RendererManifest,
  StaticPanelRenderer,
  StaticPanelType,
} from "./types"

export function isStaticPanel<T extends { type: string }>(
  item: T
): item is Extract<T, { type: StaticPanelType }> {
  return (
    item.type === "text" ||
    item.type === "placeholder" ||
    item.type === "dashlist"
  )
}

const found = import.meta.glob<{ manifest: RendererManifest }>(
  "./*/manifest.ts",
  { eager: true }
)

export const rendererManifests: ReadonlyMap<string, RendererManifest> = new Map(
  Object.values(found).map(({ manifest }) => [manifest.type, manifest])
)

export const dataRenderers: Partial<
  Record<DataPanelType, LazyExoticComponent<PanelRenderer>>
> = {}

export const staticRenderers: Partial<
  Record<StaticPanelType, LazyExoticComponent<StaticPanelRenderer>>
> = {}

for (const manifest of rendererManifests.values()) {
  if (!manifest.supported) continue
  if (isStaticPanel(manifest))
    staticRenderers[manifest.type] = lazy(guardLoad(manifest.load))
  else dataRenderers[manifest.type] = lazy(guardLoad(manifest.load))
}

export function preloadRenderer(type: string) {
  const manifest = rendererManifests.get(type)
  if (manifest?.supported) manifest.load().catch(() => undefined)
}

export function isRenderable(type: string): boolean {
  return rendererManifests.get(type)?.supported === true
}
