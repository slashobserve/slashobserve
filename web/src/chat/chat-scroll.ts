import { useEffect, useRef, type RefObject } from "react"

const stickThreshold = 80

function scrollParent(element: HTMLElement): HTMLElement | null {
  for (
    let parent = element.parentElement;
    parent !== null;
    parent = parent.parentElement
  ) {
    const overflow = getComputedStyle(parent).overflowY
    if (overflow === "auto" || overflow === "scroll") return parent
  }
  return null
}

export function useChatScroll(
  list: RefObject<HTMLElement | null>,
  target: string | null
) {
  const pinned = useRef(false)
  const scroller = useRef<HTMLElement | null>(null)

  useEffect(() => {
    const element = list.current
    if (element === null || target === null) return
    const parent = scrollParent(element)
    if (parent === null) return
    scroller.current = parent
    const atEnd = () =>
      parent.scrollHeight - parent.scrollTop - parent.clientHeight <=
      stickThreshold
    let lastTop = parent.scrollTop
    const measure = () => {
      const top = parent.scrollTop
      if (atEnd()) pinned.current = true
      else if (top < lastTop) pinned.current = false
      lastTop = top
    }
    const section =
      target === ""
        ? undefined
        : Array.from(
            element.querySelectorAll<HTMLElement>("[data-request]")
          ).find((each) => each.dataset.request === target)
    if (section === undefined) {
      parent.scrollTop = parent.scrollHeight
    } else {
      const margin = parseFloat(getComputedStyle(section).scrollMarginTop) || 0
      parent.scrollTop +=
        section.getBoundingClientRect().top -
        parent.getBoundingClientRect().top -
        margin
    }
    lastTop = parent.scrollTop
    pinned.current = atEnd()
    parent.addEventListener("scroll", measure, { passive: true })
    let lastTurn = element.lastElementChild
    let lastHeight = lastTurn?.getBoundingClientRect().height ?? 0
    const observer = new ResizeObserver(() => {
      const last = element.lastElementChild
      const height = last?.getBoundingClientRect().height ?? 0
      const grew = last !== lastTurn || height > lastHeight
      lastTurn = last
      lastHeight = height
      const busy = element.querySelector('[aria-busy="true"]') !== null
      if (pinned.current && (grew || busy)) {
        parent.scrollTop = parent.scrollHeight
      }
    })
    observer.observe(element)
    return () => {
      parent.removeEventListener("scroll", measure)
      observer.disconnect()
    }
  }, [list, target])

  return function pinToEnd() {
    pinned.current = true
    const parent = scroller.current
    if (parent !== null) parent.scrollTop = parent.scrollHeight
  }
}
