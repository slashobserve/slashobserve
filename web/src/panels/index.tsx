import { lazy, type LazyExoticComponent } from "react"

import type { DataPanelType } from "@/spec"

import type {
  PanelRenderer,
  RendererManifest,
  StaticPanelRenderer,
  StaticPanelType,
} from "./types"

type StaticManifest = Extract<RendererManifest, { type: StaticPanelType }>

function isStatic(manifest: RendererManifest): manifest is StaticManifest {
  return manifest.type === "text" || manifest.type === "placeholder"
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
  if (isStatic(manifest)) staticRenderers[manifest.type] = lazy(manifest.load)
  else dataRenderers[manifest.type] = lazy(manifest.load)
}

export function isRenderable(type: string): boolean {
  return rendererManifests.get(type)?.supported === true
}
