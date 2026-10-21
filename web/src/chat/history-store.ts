import { create } from "zustand"

type HistoryPanelState = {
  open: boolean
  setOpen: (open: boolean) => void
}

export const useHistoryPanel = create<HistoryPanelState>()((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}))
