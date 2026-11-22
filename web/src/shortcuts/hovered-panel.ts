import { create } from "zustand"

export type PanelTarget = { id: string; copy: boolean }

type HoveredPanel = {
  target: PanelTarget | null
  enter: (id: string, copy: boolean) => void
  leave: (id: string) => void
}

export const useHoveredPanel = create<HoveredPanel>()((set) => ({
  target: null,
  enter: (id, copy) => set({ target: { id, copy } }),
  leave: (id) =>
    set((state) => (state.target?.id === id ? { target: null } : state)),
}))
