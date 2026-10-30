import { useState } from "react"
import { create } from "zustand"

import type { View } from "./projection"

type SharedViews = {
  views: Record<string, View>
  set: (scope: string, view: View | null) => void
}

const useSharedViews = create<SharedViews>((state) => ({
  views: {},
  set: (scope, view) =>
    state((current) => {
      const views = { ...current.views }
      if (view === null) delete views[scope]
      else views[scope] = view
      return { views }
    }),
}))

export function useManualView(
  scope: string,
  shared: boolean
): [View | null, (view: View | null) => void] {
  const [own, setOwn] = useState<View | null>(null)
  const stored = useSharedViews((state) => state.views[scope] ?? null)
  const store = useSharedViews((state) => state.set)
  return shared ? [stored, (view) => store(scope, view)] : [own, setOwn]
}
