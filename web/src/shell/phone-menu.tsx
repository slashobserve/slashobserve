import { useState } from "react"
import { MenuIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"

import { SidebarContent } from "./sidebar"

export function PhoneMenu() {
  const [open, setOpen] = useState(false)
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Menu"
            className="hidden narrow:grid"
          />
        }
      >
        <MenuIcon aria-hidden="true" />
      </SheetTrigger>
      <SheetContent
        side="left"
        className="gap-0 bg-chrome data-[side=left]:w-[240px]"
      >
        <SheetTitle className="sr-only">Menu</SheetTitle>
        <nav
          aria-label="Main"
          className="flex min-h-0 flex-1 flex-col gap-0.5 px-2.5 pt-3.5 pb-3"
        >
          <SidebarContent collapsed={false} onNavigate={() => setOpen(false)} />
        </nav>
      </SheetContent>
    </Sheet>
  )
}
