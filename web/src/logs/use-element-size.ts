import { useLayoutEffect, useRef, useState } from "react"

function sizeOf(width: number, height: number) {
  return { width: Math.floor(width), height: Math.floor(height) }
}

export function useElementSize<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const box = element.getBoundingClientRect()
    setSize(sizeOf(box.width, box.height))
    const observer = new ResizeObserver(([entry]) => {
      setSize(sizeOf(entry.contentRect.width, entry.contentRect.height))
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return { ref, ...size }
}
