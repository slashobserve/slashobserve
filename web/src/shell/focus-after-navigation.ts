import { useEffect } from "react"
import { useRouter } from "@tanstack/react-router"

const headingWaitMs = 1500

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
    doc.querySelector<HTMLElement>("main")
  if (target) focusElement(target)
}

export function moveFocusToPage(doc: Document): () => void {
  if (!focusIsLost(doc)) return () => undefined
  const heading = () => doc.querySelector<HTMLElement>("main h1")
  const found = heading()
  if (found) {
    focusElement(found)
    return () => undefined
  }
  const observer = new MutationObserver(() => {
    const next = heading()
    if (!next) return
    stop()
    if (focusIsLost(doc)) focusElement(next)
  })
  const timer = setTimeout(() => {
    stop()
    const main = doc.querySelector<HTMLElement>("main")
    if (main && focusIsLost(doc)) focusElement(main)
  }, headingWaitMs)
  function stop() {
    observer.disconnect()
    clearTimeout(timer)
  }
  observer.observe(doc.body, { childList: true, subtree: true })
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
