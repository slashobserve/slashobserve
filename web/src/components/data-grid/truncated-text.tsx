import type { PointerEvent, ReactNode } from "react"

import { cn } from "@/lib/utils"

function titleWhenCut(event: PointerEvent<HTMLElement>) {
  const element = event.currentTarget
  if (element.scrollWidth > element.clientWidth) {
    element.title = element.textContent ?? ""
  } else {
    element.removeAttribute("title")
  }
}

export function TruncatedText({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <span className={cn("truncate", className)} onPointerEnter={titleWhenCut}>
      {children}
    </span>
  )
}
