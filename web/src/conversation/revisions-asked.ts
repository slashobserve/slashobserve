import { create } from "zustand"

type RevisionsAsked = {
  findingIds: ReadonlySet<string>
  markAsked: (findingId: string) => void
}

export const useRevisionsAsked = create<RevisionsAsked>()((set) => ({
  findingIds: new Set(),
  markAsked: (findingId) =>
    set((state) => ({ findingIds: new Set(state.findingIds).add(findingId) })),
}))
