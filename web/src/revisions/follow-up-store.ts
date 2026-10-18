import { create } from "zustand"

type FollowUpState = {
  drafts: Record<string, string>
  tracked: Record<string, string>
  watched: Record<string, string[]>
  setDraft: (dashboardId: string, text: string) => void
  track: (dashboardId: string, revisionId: string | null) => void
  watch: (dashboardId: string, revisionId: string) => void
  unwatch: (dashboardId: string, revisionId: string) => void
}

export const useFollowUpStore = create<FollowUpState>()((set) => ({
  drafts: {},
  tracked: {},
  watched: {},
  setDraft: (dashboardId, text) =>
    set((state) => ({ drafts: { ...state.drafts, [dashboardId]: text } })),
  track: (dashboardId, revisionId) =>
    set((state) => {
      const tracked = { ...state.tracked }
      if (revisionId === null) delete tracked[dashboardId]
      else tracked[dashboardId] = revisionId
      return { tracked }
    }),
  watch: (dashboardId, revisionId) =>
    set((state) => {
      const current = state.watched[dashboardId] ?? []
      if (current.includes(revisionId)) return state
      return {
        watched: { ...state.watched, [dashboardId]: [...current, revisionId] },
      }
    }),
  unwatch: (dashboardId, revisionId) =>
    set((state) => {
      const current = state.watched[dashboardId] ?? []
      if (!current.includes(revisionId)) return state
      return {
        watched: {
          ...state.watched,
          [dashboardId]: current.filter((item) => item !== revisionId),
        },
      }
    }),
}))
