import { useRef, useState } from "react"
import { MenuIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

import { SidebarSheet } from "./sidebar-sheet"

export function PhoneMenu() {
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  return (
    <>
      <Button
        ref={triggerRef}
        variant="ghost"
        size="icon-sm"
        aria-label="Menu"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        className="hidden narrow:grid"
      >
        <MenuIcon aria-hidden="true" />
      </Button>
      <SidebarSheet
        open={open}
        onOpenChange={setOpen}
        returnFocus={triggerRef}
      />
    </>
  )
}
