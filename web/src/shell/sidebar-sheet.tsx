import type { RefObject } from "react"

import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"

import { SidebarContent } from "./sidebar"

export function SidebarSheet({
  open,
  onOpenChange,
  returnFocus,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus: RefObject<HTMLElement | null>
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        finalFocus={returnFocus}
        className="gap-0 bg-chrome data-[side=left]:w-[248px]"
      >
        <SheetTitle className="sr-only">Menu</SheetTitle>
        <nav
          aria-label="Main"
          className="flex min-h-0 flex-1 flex-col gap-0.5 px-2.5 pt-3.5 pb-3"
        >
          <SidebarContent onNavigate={() => onOpenChange(false)} />
        </nav>
      </SheetContent>
    </Sheet>
  )
}
