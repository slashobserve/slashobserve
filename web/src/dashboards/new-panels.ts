import { newPanelIds, type PanelTree } from "@/observe/build"

export type SeenVersion = { version: number; spec: PanelTree }

export const noNewPanels: ReadonlySet<string> = new Set()

export function panelsAddedSince(
  seen: SeenVersion | undefined,
  next: SeenVersion
): ReadonlySet<string> {
  if (seen === undefined || next.version <= seen.version) return noNewPanels
  return newPanelIds(seen.spec, next.spec)
}
