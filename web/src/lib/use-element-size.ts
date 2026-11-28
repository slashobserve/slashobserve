import { useLayoutEffect, useState } from "react"

export type ElementSize = { width: number; height: number }

export function useElementSize<T extends HTMLElement>() {
  const [node, setNode] = useState<T | null>(null)
  const [size, setSize] = useState<ElementSize | null>(null)
  useLayoutEffect(() => {
    if (node === null) return
    const measure = (width: number, height: number) =>
      setSize({ width: Math.floor(width), height: Math.floor(height) })
    const box = node.getBoundingClientRect()
    measure(box.width, box.height)
    const observer = new ResizeObserver(([entry]) => {
      if (entry !== undefined) {
        measure(entry.contentRect.width, entry.contentRect.height)
      }
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [node])
  return {
    ref: setNode,
    node,
    size,
    width: size?.width ?? 0,
    height: size?.height ?? 0,
  }
}
