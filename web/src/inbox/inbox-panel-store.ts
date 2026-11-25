import { create } from "zustand"

import type { InboxFilter, InboxView } from "@/api/keys"

type InboxPanelState = {
  open: boolean
  view: InboxView
  filter: InboxFilter
  show: (choice?: { view?: InboxView; filter?: InboxFilter }) => void
  close: () => void
  toggle: () => void
  setView: (view: InboxView) => void
  setFilter: (filter: InboxFilter) => void
}

export const useInboxPanelStore = create<InboxPanelState>()((set) => ({
  open: false,
  view: "for_you",
  filter: "open",
  show: (choice = {}) =>
    set((state) => ({
      open: true,
      view: choice.view ?? state.view,
      filter: choice.filter ?? state.filter,
    })),
  close: () => set({ open: false }),
  toggle: () => set((state) => ({ open: !state.open })),
  setView: (view) => set({ view }),
  setFilter: (filter) => set({ filter }),
}))

export function useInboxPanelOpen(): boolean {
  return useInboxPanelStore((state) => state.open)
}

const phoneQuery = "(max-width: 899px)"

export function focusInboxBell(): void {
  const place = window.matchMedia(phoneQuery).matches ? "phone" : "top"
  document.querySelector<HTMLElement>(`[data-inbox-bell="${place}"]`)?.focus()
}
