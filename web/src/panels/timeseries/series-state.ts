import { create } from "zustand"

import { assignSlots, type SlotMap } from "../colors"

export type PanelSeriesState = {
  slots: SlotMap
  hidden: ReadonlySet<string>
}

export const emptySeriesState: PanelSeriesState = {
  slots: new Map(),
  hidden: new Set(),
}

const noneHidden: ReadonlySet<string> = new Set()

export function visibleSeries(
  hidden: ReadonlySet<string>,
  keys: readonly string[],
  legendShown: boolean
): ReadonlySet<string> {
  if (!legendShown) return noneHidden
  const present = new Set(keys)
  const kept = [...hidden].filter((key) => present.has(key))
  return kept.length === hidden.size ? hidden : new Set(kept)
}

export function commitKeys(
  state: PanelSeriesState,
  keys: readonly string[]
): PanelSeriesState {
  if (keys.length === 0) return state
  const slots = assignSlots(state.slots, keys)
  const hidden = visibleSeries(state.hidden, keys, true)
  if (slots === state.slots && hidden === state.hidden) return state
  return { slots, hidden }
}

export function toggleKey(
  state: PanelSeriesState,
  key: string
): PanelSeriesState {
  const hidden = new Set(state.hidden)
  if (!hidden.delete(key)) hidden.add(key)
  return { ...state, hidden }
}

type SeriesStore = {
  panels: Record<string, PanelSeriesState>
  commit: (id: string, keys: readonly string[]) => void
  toggle: (id: string, key: string) => void
}

export const useSeriesStore = create<SeriesStore>((set) => ({
  panels: {},
  commit: (id, keys) =>
    set((store) => {
      const current = store.panels[id] ?? emptySeriesState
      const next = commitKeys(current, keys)
      return next === current
        ? store
        : { panels: { ...store.panels, [id]: next } }
    }),
  toggle: (id, key) =>
    set((store) => ({
      panels: {
        ...store.panels,
        [id]: toggleKey(store.panels[id] ?? emptySeriesState, key),
      },
    })),
}))
