import { useCallback, useEffect, useRef, type RefObject } from "react"

import { useOpenerFocus } from "./opener-focus"

const pendingAttribute = "data-focus-pending"
const pendingMs = 2000
const announceMs = 1500

export function pendingPaneHeading(): HTMLElement | null {
  const labelledBy = document
    .querySelector(`[${pendingAttribute}]`)
    ?.getAttribute("aria-labelledby")
  return labelledBy ? document.getElementById(labelledBy) : null
}

let openingAnnouncedAt = Number.NEGATIVE_INFINITY

export function announcePaneOpening(): void {
  openingAnnouncedAt = performance.now()
}

function consumeOpening(): boolean {
  const announced = performance.now() - openingAnnouncedAt < announceMs
  openingAnnouncedAt = Number.NEGATIVE_INFINITY
  return announced
}

type Options = {
  pane: RefObject<HTMLElement | null>
  heading: RefObject<HTMLElement | null>
  opener?: () => HTMLElement | null
  onClose?: () => void
  holdClose?: () => boolean
  active?: boolean
  closeOnEscape?: boolean
  modeKey?: string
}

export function useDockFocus({
  pane,
  heading,
  opener,
  onClose,
  holdClose,
  active = true,
  closeOnEscape = true,
  modeKey = "",
}: Options) {
  const { remember, restore } = useOpenerFocus(pane, opener)
  const close = useCallback(() => {
    if (holdClose?.()) return
    restore()
    onClose?.()
  }, [holdClose, restore, onClose])

  const handled = useRef<string | null>(null)
  const wantsFocus = useRef<boolean | undefined>(undefined)
  const takes = useRef(false)
  useEffect(() => {
    if (!active) {
      handled.current = null
      takes.current = false
      wantsFocus.current = undefined
      return
    }
    const element = pane.current
    const before = handled.current
    wantsFocus.current ??= consumeOpening()
    if (before !== modeKey) {
      handled.current = modeKey
      takes.current =
        before === null
          ? wantsFocus.current
          : element?.contains(document.activeElement) !== true
    }
    if (!takes.current || !element || !heading.current) return
    remember()
    element.setAttribute(pendingAttribute, "")
    heading.current.focus()
    const timer = window.setTimeout(
      () => element.removeAttribute(pendingAttribute),
      pendingMs
    )
    return () => {
      window.clearTimeout(timer)
      element.removeAttribute(pendingAttribute)
    }
  }, [active, modeKey, pane, heading, remember])

  useEffect(() => {
    if (!active || !closeOnEscape || onClose === undefined) return
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape" || event.defaultPrevented) return
      const target = event.target
      if (!(target instanceof Node)) return
      const inside =
        pane.current?.contains(target) === true ||
        opener?.()?.contains(target) === true
      if (!inside) return
      event.preventDefault()
      close()
    }
    document.addEventListener("keydown", onKeyDown)
    return () => document.removeEventListener("keydown", onKeyDown)
  }, [active, closeOnEscape, onClose, pane, opener, close])

  return { close }
}
