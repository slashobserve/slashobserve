import { useEffect, useState, type ComponentProps } from "react"

import { cn } from "@/lib/utils"

export function hasMoreBelow(element: {
  scrollTop: number
  clientHeight: number
  scrollHeight: number
}): boolean {
  return element.scrollHeight - element.scrollTop - element.clientHeight > 1
}

export function FadingScroll({ className, ...props }: ComponentProps<"div">) {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  const [more, setMore] = useState(false)

  useEffect(() => {
    if (node === null) return
    const update = () => setMore(hasMoreBelow(node))
    update()
    node.addEventListener("scroll", update, { passive: true })
    const resize = new ResizeObserver(update)
    resize.observe(node)
    const mutation = new MutationObserver(update)
    mutation.observe(node, { childList: true, subtree: true })
    return () => {
      node.removeEventListener("scroll", update)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [node])

  return (
    <div
      {...props}
      ref={setNode}
      data-more-below={more || undefined}
      className={cn(
        "overflow-y-auto data-more-below:[mask-image:linear-gradient(to_bottom,black_calc(100%-2.25rem),transparent)]",
        className
      )}
    />
  )
}
