import { create } from "zustand"

type SelectionState = {
  dashboardId: string | null
  panelId: string | null
  select: (dashboardId: string, panelId: string) => void
  clear: () => void
}

export const useSelectionStore = create<SelectionState>()((set) => ({
  dashboardId: null,
  panelId: null,
  select: (dashboardId, panelId) => set({ dashboardId, panelId }),
  clear: () =>
    set((state) =>
      state.panelId === null ? state : { dashboardId: null, panelId: null }
    ),
}))

export function useSelectedPanel(dashboardId: string): string | null {
  return useSelectionStore((state) =>
    state.dashboardId === dashboardId ? state.panelId : null
  )
}

export type SelectionKey = "ArrowDown" | "ArrowUp" | "Home" | "End"

export function isSelectionKey(key: string): key is SelectionKey {
  return (
    key === "ArrowDown" || key === "ArrowUp" || key === "Home" || key === "End"
  )
}

export function nextSelection(
  order: readonly string[],
  current: string | null,
  key: SelectionKey
): string | null {
  if (order.length === 0) return null
  const index = current === null ? -1 : order.indexOf(current)
  if (index < 0) return order[0]
  switch (key) {
    case "Home":
      return order[0]
    case "End":
      return order[order.length - 1]
    case "ArrowDown":
      return order[Math.min(index + 1, order.length - 1)]
    case "ArrowUp":
      return order[Math.max(index - 1, 0)]
  }
}
