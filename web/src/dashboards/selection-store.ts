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
  const first = order[0]
  const last = order.at(-1)
  if (first === undefined || last === undefined) return null
  const index = current === null ? -1 : order.indexOf(current)
  if (index < 0) return first
  switch (key) {
    case "Home":
      return first
    case "End":
      return last
    case "ArrowDown":
      return order[Math.min(index + 1, order.length - 1)] ?? last
    case "ArrowUp":
      return order[Math.max(index - 1, 0)] ?? first
  }
}
