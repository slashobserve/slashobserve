import { useEffect } from "react"
import { useRouter } from "@tanstack/react-router"

const headingWaitMs = 1500
const holdMs = 5000

function focusIsLost(doc: Document): boolean {
  return doc.activeElement === null || doc.activeElement === doc.body
}

function focusElement(element: HTMLElement) {
  if (!element.hasAttribute("tabindex")) element.tabIndex = -1
  element.focus({ preventScroll: true })
}

export function focusPage(doc: Document) {
  const target =
    doc.querySelector<HTMLElement>("main h1") ??
    doc.querySelector<HTMLElement>("main") ??
    doc.querySelector<HTMLElement>("h1")
  if (target) focusElement(target)
}

export function moveFocusToPage(doc: Document): () => void {
  const departing = focusIsLost(doc) ? null : doc.activeElement
  let placed: HTMLElement | null = null
  const left = (element: Element | null) =>
    element === null || !element.isConnected
  const mayMove = () => focusIsLost(doc) && left(placed ?? departing)
  const place = (element: HTMLElement) => {
    focusElement(element)
    placed = element
  }
  const tryHeading = () => {
    const heading = doc.querySelector<HTMLElement>("main h1")
    if (heading && mayMove()) place(heading)
  }
  const observer = new MutationObserver(tryHeading)
  const fallback = setTimeout(() => {
    const main = doc.querySelector<HTMLElement>("main")
    if (main && mayMove()) place(main)
  }, headingWaitMs)
  const hold = setTimeout(stop, holdMs)
  function stop() {
    observer.disconnect()
    clearTimeout(fallback)
    clearTimeout(hold)
    doc.removeEventListener("pointerdown", stop, true)
  }
  doc.addEventListener("pointerdown", stop, true)
  observer.observe(doc.body, { childList: true, subtree: true })
  tryHeading()
  return stop
}

export function useFocusAfterNavigation() {
  const router = useRouter()
  useEffect(() => {
    let stop: () => void = () => undefined
    const unsubscribe = router.subscribe("onResolved", ({ pathChanged }) => {
      stop()
      if (pathChanged) stop = moveFocusToPage(document)
    })
    return () => {
      stop()
      unsubscribe()
    }
  }, [router])
}
