import { useCallback, useEffect, useRef, type RefObject } from "react"

const transientLayer = '[role="menu"], [role="dialog"]'

export function useOpenerFocus(
  card: RefObject<HTMLElement | null>,
  fallback?: (() => HTMLElement | null) | undefined
) {
  const opener = useRef<HTMLElement | null>(null)

  const remember = useCallback(() => {
    const active = document.activeElement
    if (
      active instanceof HTMLElement &&
      active !== document.body &&
      !card.current?.contains(active) &&
      active.closest(transientLayer) === null
    ) {
      opener.current = active
    }
  }, [card])

  useEffect(remember, [remember])

  const restore = useCallback(() => {
    const outside = (element: HTMLElement | null | undefined) =>
      element?.isConnected === true && !card.current?.contains(element)
    const remembered = opener.current
    const target = outside(remembered) ? remembered : fallback?.()
    if (outside(target)) target?.focus()
  }, [card, fallback])

  return { remember, restore }
}
