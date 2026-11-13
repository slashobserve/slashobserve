import { useEffect, useRef } from "react"

import { isEditableTarget } from "@/lib/shortcuts"

type Handlers = { focusQuery: () => void; openHelp: () => void }

const openPopup = '[role="dialog"], [role="alertdialog"]'

export function useLogsShortcuts(handlers: Handlers) {
  const latest = useRef(handlers)
  useEffect(() => {
    latest.current = handlers
  })
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (
        event.defaultPrevented ||
        event.repeat ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        isEditableTarget(event.target) ||
        document.querySelector(openPopup) !== null
      ) {
        return
      }
      const handler =
        event.key === "/"
          ? latest.current.focusQuery
          : event.key === "?"
            ? latest.current.openHelp
            : undefined
      if (handler === undefined) return
      event.preventDefault()
      handler()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])
}
