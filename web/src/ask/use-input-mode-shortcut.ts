import { useEffect, useRef } from "react"

import { isModChord } from "@/lib/shortcuts"

export function useInputModeShortcut(onToggle: () => void) {
  const latest = useRef(onToggle)
  useEffect(() => {
    latest.current = onToggle
  })
  useEffect(() => {
    function toggleBeforeTheEditorKeymap(event: KeyboardEvent) {
      if (!isModChord(event, "i") || event.isComposing) return
      event.preventDefault()
      event.stopPropagation()
      latest.current()
    }
    window.addEventListener("keydown", toggleBeforeTheEditorKeymap, true)
    return () =>
      window.removeEventListener("keydown", toggleBeforeTheEditorKeymap, true)
  }, [])
}
