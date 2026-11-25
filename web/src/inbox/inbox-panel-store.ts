import type { AnyRouter } from "@tanstack/react-router"
import { create } from "zustand"

import type { InboxFilter, InboxView } from "@/api/keys"

export type InboxChoice = {
  view?: InboxView
  filter?: InboxFilter
  item?: number
}

type InboxPanelState = {
  open: boolean
  view: InboxView
  filter: InboxFilter
  item: number | null
  openings: number
  pending: InboxChoice | null
  show: (choice?: InboxChoice) => void
  showAfterNavigation: (choice: InboxChoice) => void
  close: () => void
  toggle: () => void
  setView: (view: InboxView) => void
  setFilter: (filter: InboxFilter) => void
}

export const useInboxPanelStore = create<InboxPanelState>()((set, get) => ({
  open: false,
  view: "for_you",
  filter: "open",
  item: null,
  openings: 0,
  pending: null,
  show: (choice = {}) =>
    set((state) => ({
      open: true,
      view: choice.view ?? state.view,
      filter: choice.filter ?? state.filter,
      item: choice.item ?? null,
      openings: state.openings + 1,
      pending: null,
    })),
  showAfterNavigation: (choice) => set({ pending: choice }),
  close: () => set({ open: false, item: null }),
  toggle: () => {
    if (get().open) get().close()
    else get().show()
  },
  setView: (view) => set({ view }),
  setFilter: (filter) => set({ filter }),
}))

export function closeInboxIfOpen(): boolean {
  const { open, close } = useInboxPanelStore.getState()
  if (open) close()
  return open
}

export function toggleInboxFromKeys(): void {
  const { open, close, show } = useInboxPanelStore.getState()
  if (!open) {
    show()
    return
  }
  close()
  returnFocusFromInbox()
}

export function useInboxPanelOpen(): boolean {
  return useInboxPanelStore((state) => state.open)
}

const redirectPath = "/inbox"

export function inboxAfterNavigation(to: string, pathChanged: boolean): void {
  if (to === redirectPath) return
  const { pending, open, show, close } = useInboxPanelStore.getState()
  if (pending !== null) show(pending)
  else if (open && pathChanged) close()
}

export function followNavigationWithInbox(router: AnyRouter) {
  router.subscribe("onResolved", ({ toLocation, pathChanged }) =>
    inboxAfterNavigation(toLocation.pathname, pathChanged)
  )
}

function shown(element: HTMLElement): boolean {
  return (
    element.isConnected &&
    (element.checkVisibility?.() ?? element.offsetParent !== null)
  )
}

export function returnFocusFromInbox(): void {
  Array.from(document.querySelectorAll<HTMLElement>("[data-inbox-bell]"))
    .find(shown)
    ?.focus()
}
