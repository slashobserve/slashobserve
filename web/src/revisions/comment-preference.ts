import { create } from "zustand"

const storageKey = "slashobserve-revise-on-comment"

function read(): boolean {
  try {
    return localStorage.getItem(storageKey) !== "off"
  } catch {
    return true
  }
}

function write(asked: boolean) {
  try {
    if (asked) localStorage.removeItem(storageKey)
    else localStorage.setItem(storageKey, "off")
  } catch {
    return
  }
}

type PreferenceState = {
  asked: boolean
  setAsked: (asked: boolean) => void
}

export const useCommentRevisionPreference = create<PreferenceState>()(
  (set) => ({
    asked: read(),
    setAsked: (asked) => {
      write(asked)
      set({ asked })
    },
  })
)
