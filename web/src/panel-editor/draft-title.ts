import { create } from "zustand"

type DraftTitles = {
  titles: Record<string, string>
  show: (panelId: string, title: string | undefined) => void
}

export const useDraftTitles = create<DraftTitles>()((set) => ({
  titles: {},
  show: (panelId, title) =>
    set((state) => {
      const shown = title?.trim() ? title : undefined
      if (state.titles[panelId] === shown) return state
      const titles = { ...state.titles }
      if (shown === undefined) Reflect.deleteProperty(titles, panelId)
      else titles[panelId] = shown
      return { titles }
    }),
}))

export function useDraftTitle(panelId: string | null): string | undefined {
  return useDraftTitles((state) =>
    panelId === null ? undefined : state.titles[panelId]
  )
}
