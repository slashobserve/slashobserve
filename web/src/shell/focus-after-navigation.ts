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
    doc.querySelector<HTMLElement>("main") ??
    doc.querySelector<HTMLElement>("h1")
  if (target) focusElement(target)
}

export function moveFocusToPage(doc: Document): () => void {
  const departing = focusIsLost(doc) ? null : doc.activeElement
  if (departing === null) return waitForHeading(doc, () => true)
  return waitForHeading(doc, () => !departing.isConnected)
}

function waitForHeading(doc: Document, mayMove: () => boolean): () => void {
  const heading = () => doc.querySelector<HTMLElement>("main h1")
  const ready = () => mayMove() && focusIsLost(doc)
  const found = heading()
  if (found && ready()) {
    focusElement(found)
    return () => undefined
  }
  const observer = new MutationObserver(() => {
    const next = heading()
    if (!next || !ready()) return
    stop()
    focusElement(next)
  })
  const timer = setTimeout(() => {
    stop()
    const main = doc.querySelector<HTMLElement>("main")
    if (main && ready()) focusElement(main)
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
