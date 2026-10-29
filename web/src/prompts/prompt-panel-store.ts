import { create } from "zustand"

export type PromptPanelTarget =
  | { kind: "create"; body: string; returnFocusId: string }
  | { kind: "edit"; promptId: string; returnFocusId: string }

type PromptPanelState = {
  target: PromptPanelTarget | null
  openPanel: (target: PromptPanelTarget) => void
  closePanel: () => void
}

export const usePromptPanel = create<PromptPanelState>()((set) => ({
  target: null,
  openPanel: (target) => set({ target }),
  closePanel: () => set({ target: null }),
}))
