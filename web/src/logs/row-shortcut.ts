import { useEffect } from "react"

function firstLogRow(root: ParentNode): HTMLElement | null {
  return root.querySelector<HTMLElement>("ul[data-logs-count] > li > button")
}

export function useFirstRowShortcut() {
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "j" || event.defaultPrevented) return
      if (event.ctrlKey || event.metaKey || event.altKey) return
      if (event.target !== document.body) return
      const row = firstLogRow(document)
      if (row === null) return
      event.preventDefault()
      row.focus()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])
}
