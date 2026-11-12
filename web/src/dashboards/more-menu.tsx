import {
  CopyIcon,
  DownloadIcon,
  EllipsisIcon,
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

import { moreMenuButtonId } from "./edit/layout-store"

type Props = {
  mayEdit: boolean
  detailsBlockedBy: string | null
  duplicateBlockedBy: string | null
  deleteBlockedBy: string | null
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
  detailsBlockedBy,
  duplicateBlockedBy,
  deleteBlockedBy,
  onEditDetails,
  onDuplicate,
  onDelete,
  onExport,
  onTvMode,
  onTvModeFolder,
  cursorSync,
  onCursorSyncChange,
}: Props) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        id={moreMenuButtonId}
        render={
          <button
            type="button"
            aria-label="More: edit details, export, duplicate, TV mode, TV mode for this folder, chart hover, delete"
            className="grid size-[30px] place-items-center rounded-control outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-muted"
          />
        }
      >
        <EllipsisIcon aria-hidden="true" className="size-4" strokeWidth={2.4} />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="min-w-max"
        finalFocus={() =>
          !focusMovedOutOfMenu(document.activeElement, document.body)
        }
      >
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
