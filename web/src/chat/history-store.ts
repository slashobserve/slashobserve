import { create } from "zustand"

export type HistoryTab = "chats" | "imports"

type HistoryPanelState = {
  open: boolean
  tab: HistoryTab
  setOpen: (open: boolean) => void
  setTab: (tab: HistoryTab) => void
}

export const useHistoryPanel = create<HistoryPanelState>()((set) => ({
  open: false,
  tab: "chats",
  setOpen: (open) => set({ open }),
  setTab: (tab) => set({ tab }),
}))
