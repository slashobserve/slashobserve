import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
} from "react"

import type { AnchorsModule, Rect } from "./anchors"
import { rendererManifests } from "./index"

export type PublishedLayout = { type: string; layout: unknown }

export type AnchorLayoutStore = {
  publish: (type: string, layout: unknown | null) => void
  current: () => PublishedLayout | null
  subscribe: (listener: () => void) => () => void
}

export function createAnchorLayoutStore(): AnchorLayoutStore {
  let current: PublishedLayout | null = null
  const listeners = new Set<() => void>()
  return {
    publish(type, layout) {
      if (layout === null && current?.type !== type) return
      const next = layout === null ? null : { type, layout }
      if (current?.type === next?.type && current?.layout === next?.layout) {
        return
      }
      current = next
      for (const listener of listeners) listener()
    },
    current: () => current,
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
}

const AnchorLayoutContext = createContext<AnchorLayoutStore | null>(null)

export const AnchorLayoutProvider = AnchorLayoutContext.Provider

const ignore: AnchorLayoutStore["publish"] = () => undefined

function useAnchorLayout(): AnchorLayoutStore["publish"] {
  return useContext(AnchorLayoutContext)?.publish ?? ignore
}

export function useAnchorPublisher(type: string): (layout: unknown) => void {
  const publish = useAnchorLayout()
  useEffect(() => () => publish(type, null), [publish, type])
  return useCallback(
    (layout: unknown) => publish(type, layout),
    [publish, type]
  )
}

export function surfaceOf(element: Element): Element {
  return element.closest("[data-canvas-surface]") ?? element
}

export function rectWithin(element: Element, root: Element): Rect {
  const box = element.getBoundingClientRect()
  const origin = root.getBoundingClientRect()
  return {
    x: box.left - origin.left,
    y: box.top - origin.top,
    width: box.width,
    height: box.height,
  }
}

const noStore: AnchorLayoutStore = {
  publish: ignore,
  current: () => null,
  subscribe: () => () => undefined,
}

export function usePublished(
  store: AnchorLayoutStore | null
): PublishedLayout | null {
  const source = store ?? noStore
  return useSyncExternalStore(source.subscribe, source.current, source.current)
}

const loaded = new Map<string, AnchorsModule | null>()

function loadAnchors(type: string): Promise<AnchorsModule | null> {
  const load = rendererManifests.get(type)?.anchors
  if (load === undefined) return Promise.resolve(null)
  return load().catch(() => null)
}

export function useAnchorsModule(type: string | null): AnchorsModule | null {
  const [, setVersion] = useState(0)
  useEffect(() => {
    if (type === null || loaded.has(type)) return
    let live = true
    void loadAnchors(type).then((module) => {
      loaded.set(type, module)
      if (live) setVersion((version) => version + 1)
    })
    return () => {
      live = false
    }
  }, [type])
  return type === null ? null : (loaded.get(type) ?? null)
}
