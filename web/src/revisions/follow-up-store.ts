import { create } from "zustand"

type FollowUpState = {
  drafts: Record<string, string>
  tracked: Record<string, string>
  setDraft: (dashboardId: string, text: string) => void
  track: (dashboardId: string, revisionId: string | null) => void
}

export const useFollowUpStore = create<FollowUpState>()((set) => ({
  drafts: {},
  tracked: {},
  setDraft: (dashboardId, text) =>
    set((state) => ({ drafts: { ...state.drafts, [dashboardId]: text } })),
  track: (dashboardId, revisionId) =>
    set((state) => {
      const tracked = { ...state.tracked }
      if (revisionId === null) delete tracked[dashboardId]
      else tracked[dashboardId] = revisionId
      return { tracked }
    }),
}))
