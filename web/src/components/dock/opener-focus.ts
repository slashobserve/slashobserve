import { useCallback, useEffect, useRef, type RefObject } from "react"

export function useOpenerFocus(card: RefObject<HTMLElement | null>) {
  const opener = useRef<HTMLElement | null>(null)

  const remember = useCallback(() => {
    const active = document.activeElement
    if (
      active instanceof HTMLElement &&
      active !== document.body &&
      !card.current?.contains(active)
    ) {
      opener.current = active
    }
  }, [card])

  useEffect(remember, [remember])

  const restore = useCallback(() => {
    const from = opener.current
    if (from === null) return
    requestAnimationFrame(() => {
      if (from.isConnected) from.focus()
    })
  }, [])

  return { remember, restore }
}
