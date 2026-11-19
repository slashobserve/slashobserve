const paneSelector = "[data-export-pane]"
const waitFrames = 60

export type SavedScroll = { path: string; window: number; pane: number }

export function saveScroll(): SavedScroll {
  return {
    path: location.pathname,
    window: window.scrollY,
    pane: document.querySelector<HTMLElement>(paneSelector)?.scrollTop ?? 0,
  }
}

export function restoreScroll(saved: SavedScroll) {
  if (location.pathname !== saved.path) return
  const attempt = (left: number) => {
    const pane = document.querySelector<HTMLElement>(paneSelector)
    const tall =
      pane === null || pane.scrollHeight - pane.clientHeight >= saved.pane
    if (!tall && left > 0) {
      requestAnimationFrame(() => attempt(left - 1))
      return
    }
    if (pane) pane.scrollTop = saved.pane
    window.scrollTo(0, saved.window)
  }
  attempt(waitFrames)
}
