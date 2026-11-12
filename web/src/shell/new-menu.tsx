import { useNavigate } from "@tanstack/react-router"
import {
  NotebookTextIcon,
  PlusIcon,
  TerminalIcon,
  UploadIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { observationsLink } from "@/lib/links"

export function NewMenu({
  collapsed,
  onNavigate,
}: {
  collapsed: boolean
  onNavigate?: () => void
}) {
  const navigate = useNavigate()
  if (collapsed) return null

  function go(open: () => Promise<void>) {
    onNavigate?.()
    void open()
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="New"
            title="New"
          />
        }
      >
        <PlusIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuItem
          onClick={() => go(() => navigate(observationsLink({ new: "1" })))}
        >
          <NotebookTextIcon aria-hidden="true" />
          New observation
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => go(() => navigate({ to: "/" }))}>
          <TerminalIcon aria-hidden="true" />
          Ask Observe Agent
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => go(() => navigate({ to: "/import" }))}>
          <UploadIcon aria-hidden="true" />
          Import a dashboard
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
