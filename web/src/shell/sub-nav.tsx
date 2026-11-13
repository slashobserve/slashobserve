import type { ReactNode } from "react"

export const subNavLinkClass =
  "flex h-7 items-center gap-2 rounded-control border border-transparent px-2 text-[13px] text-foreground no-underline outline-none hover:bg-nav-active focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-nav-active"

export const subNavMoreClass =
  "flex h-7 items-center rounded-control px-2 text-xs text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"

export function SubNavList({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <ul
      aria-label={label}
      className="ml-[18px] flex flex-col gap-0.5 border-l pl-1.5"
    >
      {children}
    </ul>
  )
}
