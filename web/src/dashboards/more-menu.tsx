import { useRef } from "react"
import { useNavigate, useRouteContext } from "@tanstack/react-router"
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
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { focusMovedOutOfMenu } from "@/lib/focus"
import { roleAtLeast } from "@/lib/roles"
import { modChordLabel } from "@/lib/shortcuts"
import type { CursorSync } from "@/panels/shared-cursor"
import { useSignOut } from "@/shell/use-sign-out"

import { cursorSyncOptions, isCursorSync } from "./cursor-sync"

import { findFirstMoveHandle } from "./edit/edit-controls"
import { editLayoutButtonId } from "./edit/layout-store"

type Props = {
  mayEdit: boolean
  canEditLayout: boolean
  layoutBlockedByActivity: boolean
  pinned: boolean
  onEditLayout: () => void
  onExport: () => void
  cursorSync: CursorSync
  onCursorSyncChange: (mode: CursorSync) => void
}

export function MoreMenu({
  mayEdit,
  canEditLayout,
  layoutBlockedByActivity,
  pinned,
  onEditLayout,
  onExport,
  cursorSync,
  onCursorSyncChange,
}: Props) {
  const navigate = useNavigate()
  const { principal } = useRouteContext({ from: "/_authed" })
  const { signOut, pending } = useSignOut()
  const startedLayout = useRef(false)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        id={editLayoutButtonId}
        render={
          <button
            type="button"
            aria-label="More: edit layout, export, chart hover, API keys, sign out"
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
          if (focusMovedOutOfMenu(document.activeElement, document.body)) {
            return false
          }
          return (started && findFirstMoveHandle()) || true
        }}
      >
        {mayEdit && (canEditLayout || layoutBlockedByActivity) ? (
          <DropdownMenuItem
            disabled={pinned || !canEditLayout}
            onClick={() => {
              startedLayout.current = true
              onEditLayout()
            }}
          >
            <LayoutGridIcon aria-hidden="true" />
            {pinned
              ? "Edit layout (open the latest version first)"
              : canEditLayout
                ? "Edit layout"
                : "Edit layout (close activity first)"}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onClick={onExport}>
          <DownloadIcon aria-hidden="true" />
          Export spec JSON
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center gap-4">
            Chart hover
            <DropdownMenuShortcut>
              {modChordLabel(navigator.userAgent, "O")}
            </DropdownMenuShortcut>
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={cursorSync}
            onValueChange={(value) => {
              if (isCursorSync(value)) onCursorSyncChange(value)
            }}
          >
            {cursorSyncOptions.map((option) => (
              <DropdownMenuRadioItem
                key={option.value}
                value={option.value}
                closeOnClick
                inset
              >
                {option.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        {roleAtLeast(principal.role, "admin") ? (
          <DropdownMenuItem onClick={() => void navigate({ to: "/keys" })}>
            <KeyIcon aria-hidden="true" />
            API keys
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem disabled={pending} onClick={() => void signOut()}>
          <LogOutIcon aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
