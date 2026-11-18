import { create } from "zustand"

type ExportMode = "print" | "image"

type ExportState = {
  mode: ExportMode | null
  start: (mode: ExportMode) => void
  finish: () => void
}

export const useExportStore = create<ExportState>()((set) => ({
  mode: null,
  start: (mode) => set({ mode }),
  finish: () => set({ mode: null }),
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
