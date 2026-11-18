import type { RefObject } from "react"
import {
  BookmarkIcon,
  FolderSearchIcon,
  LinkIcon,
  MoreHorizontalIcon,
} from "lucide-react"
import { toast } from "sonner"

import { CopyButton } from "@/components/copy-button"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { SavedSearchItems } from "@/saved-searches/saved-search-items"
import { SavedSearchesMenu } from "@/saved-searches/saved-searches-menu"
import { TopRowActions } from "@/shell/top-row-actions"

type Props = {
  range: { from: string; to: string }
  onRangeChange: (next: { from: string; to: string }) => void
  canSave: boolean
  saveTrigger: RefObject<HTMLButtonElement | null>
  moreTrigger: RefObject<HTMLButtonElement | null>
  onSave: () => void
}

const notRunText = "Run a query first."

async function copyLink() {
  try {
    await navigator.clipboard.writeText(window.location.href)
    toast.success("Link copied")
  } catch {
    toast.error("Could not copy. Select the text and press Ctrl+C or Cmd+C.")
  }
}

function MoreActions({
  canSave,
  moreTrigger,
  onSave,
}: Pick<Props, "canSave" | "moreTrigger" | "onSave">) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            ref={moreTrigger}
            variant="ghost"
            size="icon"
            aria-label="More actions"
            className="hidden size-11 narrow:inline-flex"
          />
        }
      >
        <MoreHorizontalIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuItem
          disabled={!canSave}
          title={canSave ? undefined : notRunText}
          className="min-h-11"
          onClick={onSave}
        >
          <BookmarkIcon aria-hidden="true" />
          Save search
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="min-h-11">
            <FolderSearchIcon aria-hidden="true" />
            Saved searches
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="w-64">
            <SavedSearchItems kind="metrics" />
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem className="min-h-11" onClick={() => void copyLink()}>
          <LinkIcon aria-hidden="true" />
          Copy link
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function MetricsPageActions({
  range,
  onRangeChange,
  canSave,
  saveTrigger,
  moreTrigger,
  onSave,
}: Props) {
  return (
    <TopRowActions>
      <div className="flex items-center gap-2">
        <span className="contents narrow:[&_button]:h-11">
          <TimeRangeControl value={range} onChange={onRangeChange} />
        </span>
        <span aria-hidden="true" className="h-5 w-px bg-border narrow:hidden" />
        <span className="contents narrow:hidden">
          <SavedSearchesMenu kind="metrics" />
          <Button
            ref={saveTrigger}
            type="button"
            variant="ghost"
            disabled={!canSave}
            title={canSave ? undefined : notRunText}
            onClick={onSave}
          >
            <BookmarkIcon aria-hidden="true" />
            <span className="@max-[780px]/top:sr-only">Save search</span>
          </Button>
          <CopyButton
            text={window.location.href}
            label="Copy link"
            variant="ghost"
            size="icon"
          />
        </span>
        <MoreActions
          canSave={canSave}
          moreTrigger={moreTrigger}
          onSave={onSave}
        />
      </div>
    </TopRowActions>
  )
}
