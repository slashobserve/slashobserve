import { useRef } from "react"
import {
  CopyIcon,
  DownloadIcon,
  EllipsisIcon,
  LayoutGridIcon,
  PencilIcon,
  TvIcon,
  TvMinimalPlayIcon,
  Trash2Icon,
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
import { modChordLabel } from "@/lib/shortcuts"
import type { CursorSync } from "@/panels/shared-cursor"

import { cursorSyncOptions, isCursorSync } from "./cursor-sync"

import { findFirstMoveHandle } from "./edit/edit-controls"
import { editLayoutButtonId } from "./edit/layout-store"

type Props = {
  mayEdit: boolean
  canEditLayout: boolean
  layoutBlockedByActivity: boolean
  pinned: boolean
  detailsBlockedBy: string | null
  duplicateBlockedBy: string | null
  deleteBlockedBy: string | null
  onEditLayout: () => void
  onEditDetails: () => void
  onDuplicate: (() => void) | null
  onDelete: () => void
  onExport: () => void
  onTvMode: () => void
  onTvModeFolder: (() => void) | null
  cursorSync: CursorSync
  onCursorSyncChange: (mode: CursorSync) => void
}

export function MoreMenu({
  mayEdit,
  canEditLayout,
  layoutBlockedByActivity,
  pinned,
  detailsBlockedBy,
  duplicateBlockedBy,
  deleteBlockedBy,
  onEditLayout,
  onEditDetails,
  onDuplicate,
  onDelete,
  onExport,
  onTvMode,
  onTvModeFolder,
  cursorSync,
  onCursorSyncChange,
}: Props) {
  const startedLayout = useRef(false)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        id={editLayoutButtonId}
        render={
          <button
            type="button"
            aria-label="More: edit layout, edit details, export, duplicate, TV mode, TV mode for this folder, chart hover, delete"
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
        {mayEdit ? (
          <DropdownMenuItem
            disabled={detailsBlockedBy !== null}
            onClick={onEditDetails}
          >
            <PencilIcon aria-hidden="true" />
            {detailsBlockedBy === null
              ? "Edit details"
              : `Edit details (${detailsBlockedBy})`}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onClick={onExport}>
          <DownloadIcon aria-hidden="true" />
          Export spec JSON
        </DropdownMenuItem>
        {onDuplicate !== null ? (
          <DropdownMenuItem
            disabled={duplicateBlockedBy !== null}
            onClick={onDuplicate}
          >
            <CopyIcon aria-hidden="true" />
            {duplicateBlockedBy === null
              ? "Duplicate..."
              : `Duplicate (${duplicateBlockedBy})`}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onTvMode}>
          <TvIcon aria-hidden="true" />
          TV mode
          <DropdownMenuShortcut aria-hidden="true">d k</DropdownMenuShortcut>
        </DropdownMenuItem>
        {onTvModeFolder !== null ? (
          <DropdownMenuItem onClick={onTvModeFolder}>
            <TvMinimalPlayIcon aria-hidden="true" />
            TV mode for this folder
          </DropdownMenuItem>
        ) : null}
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
        {mayEdit ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              disabled={deleteBlockedBy !== null}
              onClick={onDelete}
            >
              <Trash2Icon aria-hidden="true" />
              {deleteBlockedBy === null
                ? "Delete dashboard..."
                : `Delete dashboard (${deleteBlockedBy})`}
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
