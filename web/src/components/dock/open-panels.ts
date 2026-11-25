import { useInboxPanelStore } from "@/inbox/inbox-panel-store"

const logicallyOpen = new Set<string>()
const shownInstances = new Map<string, number>()

export function panelShown(key: string): boolean {
  const fresh = !logicallyOpen.has(key)
  logicallyOpen.add(key)
  shownInstances.set(key, (shownInstances.get(key) ?? 0) + 1)
  return fresh
}

export function panelLeft(key: string, unmounting: boolean): void {
  const remaining = (shownInstances.get(key) ?? 1) - 1
  if (remaining > 0) shownInstances.set(key, remaining)
  else shownInstances.delete(key)
  const mightRemount = unmounting && useInboxPanelStore.getState().open
  if (!mightRemount) logicallyOpen.delete(key)
}

function forgetPanelsThatNeverReturned(): void {
  for (const key of logicallyOpen) {
    if (!shownInstances.has(key)) logicallyOpen.delete(key)
  }
}

useInboxPanelStore.subscribe((state, previous) => {
  if (previous.open && !state.open) forgetPanelsThatNeverReturned()
})

export function resetOpenPanels(): void {
  logicallyOpen.clear()
  shownInstances.clear()
}
