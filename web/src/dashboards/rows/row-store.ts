import { create } from "zustand"

type RowState = {
  collapsed: Record<string, Record<string, boolean>>
  toggle: (dashboardId: string, rowId: string, initial: boolean) => void
}

export const useRowStore = create<RowState>()((set) => ({
  collapsed: {},
  toggle: (dashboardId, rowId, initial) =>
    set((state) => {
      const rows = state.collapsed[dashboardId]
      return {
        collapsed: {
          ...state.collapsed,
          [dashboardId]: { ...rows, [rowId]: !(rows?.[rowId] ?? initial) },
        },
      }
    }),
}))
