import { useEffect, useState, type ComponentProps } from "react"
import { ChevronDownIcon } from "lucide-react"

import { cn } from "@/lib/utils"

export function hasMoreBelow(element: {
  scrollTop: number
  clientHeight: number
  scrollHeight: number
}): boolean {
  return element.scrollHeight - element.scrollTop - element.clientHeight > 1
}

export function ScrollWithMoreCue({
  className,
  ...props
}: Omit<ComponentProps<"div">, "ref">) {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  const [more, setMore] = useState(false)

  useEffect(() => {
    if (node === null) return
    const update = () => setMore(hasMoreBelow(node))
    const resize = new ResizeObserver(update)
    const watchChildren = () => {
      resize.disconnect()
      resize.observe(node)
      for (const child of Array.from(node.children)) resize.observe(child)
    }
    const mutation = new MutationObserver(() => {
      watchChildren()
      update()
    })
    watchChildren()
    update()
    node.addEventListener("scroll", update, { passive: true })
    mutation.observe(node, { childList: true, subtree: true })
    void document.fonts?.ready.then(update)
    return () => {
      node.removeEventListener("scroll", update)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [node])

  return (
    <>
      <div
        {...props}
        ref={setNode}
        data-more-below={more || undefined}
        className={cn("overflow-y-auto", className)}
      />
      {more ? (
        <div
          aria-hidden="true"
          data-more-cue=""
          className="flex h-5 shrink-0 items-center justify-center border-t border-border text-muted-foreground"
        >
          <ChevronDownIcon className="size-3.5" />
        </div>
      ) : null}
    </>
  )
}
