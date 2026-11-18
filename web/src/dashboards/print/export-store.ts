import { create } from "zustand"

import { restoreScroll, saveScroll, type SavedScroll } from "./scroll"

type ExportMode = "print" | "image"

type ExportState = {
  mode: ExportMode | null
  scroll: SavedScroll | null
  start: (mode: ExportMode) => void
  finish: () => void
}

export const useExportStore = create<ExportState>()((set, get) => ({
  mode: null,
  scroll: null,
  start: (mode) => set({ mode, scroll: saveScroll() }),
  finish: () => {
    const { mode, scroll } = get()
    set({ mode: null, scroll: null })
    if (mode !== null && scroll !== null) restoreScroll(scroll)
  },
}))

export function useExporting(): boolean {
  return useExportStore((state) => state.mode !== null)
}

let sheets = 0

export function holdSheet(): () => void {
  sheets += 1
  return () => {
    sheets -= 1
    setTimeout(() => {
      if (sheets === 0) useExportStore.getState().finish()
    }, 0)
  }
}
