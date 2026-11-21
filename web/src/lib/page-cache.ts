// Chrome keeps a cached page's open streams, and so their HTTP/1.1 connections, until it evicts the page.
export function closeWhileCached(
  close: () => void,
  reopen?: () => void
): () => void {
  const onHide = () => close()
  const onShow = (event: PageTransitionEvent) => {
    if (event.persisted) reopen?.()
  }
  window.addEventListener("pagehide", onHide)
  window.addEventListener("pageshow", onShow)
  return () => {
    window.removeEventListener("pagehide", onHide)
    window.removeEventListener("pageshow", onShow)
  }
}
