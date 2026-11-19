import { useEffect, useState, type RefObject } from "react"

import { useExporting } from "./print/export-store"

export function useInView(ref: RefObject<Element | null>): {
  seen: boolean
  inView: boolean
} {
  const [inView, setInView] = useState(false)
  const [seen, setSeen] = useState(false)
  const exporting = useExporting()

  useEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new IntersectionObserver(
      (entries) => {
        const latest = entries.at(-1)
        if (!latest) return
        setInView(latest.isIntersecting)
        if (latest.isIntersecting) setSeen(true)
      },
      { rootMargin: "200px" }
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])

  return { seen: seen || exporting, inView: inView || exporting }
}
