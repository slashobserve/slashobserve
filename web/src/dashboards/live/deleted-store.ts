import { create } from "zustand"

type DeletedDashboards = {
  deletedBy: Record<string, string>
  heldDrafts: Record<string, true>
  markDeleted: (dashboardId: string, by: string) => void
  holdDraft: (dashboardId: string, held: boolean) => void
}

export const useDeletedDashboards = create<DeletedDashboards>()((set) => ({
  deletedBy: {},
  heldDrafts: {},
  markDeleted: (dashboardId, by) =>
    set((state) =>
      state.deletedBy[dashboardId] === undefined || by !== ""
        ? { deletedBy: { ...state.deletedBy, [dashboardId]: by } }
        : state
    ),
  holdDraft: (dashboardId, held) =>
    set((state) => {
      if ((state.heldDrafts[dashboardId] === true) === held) return state
      const heldDrafts = { ...state.heldDrafts }
      if (held) heldDrafts[dashboardId] = true
      else Reflect.deleteProperty(heldDrafts, dashboardId)
      return { heldDrafts }
    }),
}))

export function useDeletedBy(dashboardId: string): string | undefined {
  return useDeletedDashboards((state) => state.deletedBy[dashboardId])
}

export function deletedText(by: string | undefined): string {
  return by ? `${by} deleted this dashboard.` : "This dashboard was deleted."
}
