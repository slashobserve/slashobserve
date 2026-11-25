import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { create } from "zustand"

import type { InputMode } from "@/ask/input-mode"

import { savedSearchLocation } from "./links"

type SavedOpens = { count: number; bump: () => void }

export const useSavedOpens = create<SavedOpens>()((set) => ({
  count: 0,
  bump: () => set((state) => ({ count: state.count + 1 })),
}))

export function useOpenSavedSearch() {
  const navigate = useNavigate()
  const bump = useSavedOpens((state) => state.bump)
  return (saved: Parameters<typeof savedSearchLocation>[0]) => {
    bump()
    void navigate(savedSearchLocation(saved))
  }
}

export function useLeaveAskOnSavedOpen(
  mode: InputMode,
  setMode: (next: InputMode, remember?: boolean) => void
) {
  const opens = useSavedOpens((state) => state.count)
  const [seen, setSeen] = useState(opens)
  if (seen === opens) return
  setSeen(opens)
  if (mode === "ask") setMode("code", false)
}
