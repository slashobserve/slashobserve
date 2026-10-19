import { useRef } from "react"
import { useNavigate } from "@tanstack/react-router"
import {
  DownloadIcon,
  EllipsisIcon,
  KeyIcon,
  LayoutGridIcon,
  LogOutIcon,
} from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useSignOut } from "@/shell/use-sign-out"

import { findFirstMoveHandle } from "./edit/edit-controls"
import { editLayoutButtonId } from "./edit/layout-store"

type Props = {
  canEditLayout: boolean
  pinned: boolean
  onEditLayout: () => void
  onExport: () => void
}

export function MoreMenu({
  canEditLayout,
  pinned,
  onEditLayout,
  onExport,
}: Props) {
  const navigate = useNavigate()
  const { signOut, pending } = useSignOut()
  const startedLayout = useRef(false)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        id={editLayoutButtonId}
        render={
          <button
            type="button"
            aria-label="More: edit layout, export, API keys, sign out"
            className="grid size-[30px] place-items-center rounded-control outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted"
          />
        }
      >
        <EllipsisIcon aria-hidden="true" className="size-4" strokeWidth={2.4} />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="min-w-max"
        finalFocus={() => {
          const started = startedLayout.current
          startedLayout.current = false
          const active = document.activeElement
          const idle =
            active === null ||
            active === document.body ||
            active.closest('[role="menu"]') !== null
          return (started && idle && findFirstMoveHandle()) || true
        }}
      >
        {canEditLayout ? (
          <DropdownMenuItem
            disabled={pinned}
            onClick={() => {
              startedLayout.current = true
              onEditLayout()
            }}
          >
            <LayoutGridIcon aria-hidden="true" />
            {pinned
              ? "Edit layout (open the latest version first)"
              : "Edit layout"}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onClick={onExport}>
          <DownloadIcon aria-hidden="true" />
          Export spec JSON
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => void navigate({ to: "/keys" })}>
          <KeyIcon aria-hidden="true" />
          API keys
        </DropdownMenuItem>
        <DropdownMenuItem disabled={pending} onClick={() => void signOut()}>
          <LogOutIcon aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
