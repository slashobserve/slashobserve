import { useEffect, type RefObject } from "react"

import type { SpotTarget } from "@/conversation/send-plan"

import type { ContextChip } from "./context-chips"

export type ChatPanel = { id: string; title: string; onRemove: () => void }

export function panelChips(panel: ChatPanel | null): ContextChip[] {
  if (panel === null) return []
  return [
    {
      id: "panel",
      label: panel.title === "" ? "Untitled panel" : panel.title,
      onRemove: panel.onRemove,
    },
  ]
}

export function panelTarget(panel: ChatPanel | null): SpotTarget | undefined {
  return panel === null ? undefined : { panelId: panel.id }
}

export function useFocusOnPanel(
  panel: ChatPanel | null,
  field: RefObject<HTMLTextAreaElement | null>
) {
  const panelId = panel?.id
  useEffect(() => {
    if (panelId !== undefined) field.current?.focus()
  }, [panelId, field])
}
