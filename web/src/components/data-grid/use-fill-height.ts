import { useLayoutEffect, useState, type RefObject } from "react"

import { fillHeight } from "./layout"

export function useFillHeight(
  ref: RefObject<HTMLElement | null>,
  active: boolean
): number | undefined {
  const [height, setHeight] = useState<number>()

  useLayoutEffect(() => {
    const element = ref.current
    if (!active || element === null) return
    function fit() {
      if (element === null) return
      setHeight(
        fillHeight(window.innerHeight, element.getBoundingClientRect().top)
      )
    }
    fit()
    window.addEventListener("resize", fit)
    return () => window.removeEventListener("resize", fit)
  }, [ref, active])

  return active ? height : undefined
}
