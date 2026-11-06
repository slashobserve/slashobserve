import { create } from "zustand"

type HoveredPanel = {
  id: string | null
  enter: (id: string) => void
  leave: (id: string) => void
}

export const useHoveredPanel = create<HoveredPanel>()((set) => ({
  id: null,
  enter: (id) => set({ id }),
  leave: (id) => set((state) => (state.id === id ? { id: null } : state)),
}))
