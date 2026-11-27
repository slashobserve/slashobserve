import type { RefObject } from "react"
import {
  BookmarkIcon,
  FolderSearchIcon,
  LinkIcon,
  MoreHorizontalIcon,
  RadioIcon,
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
import {
  TimeRangeControl,
  type TimeRangeValue,
} from "@/dashboards/time-range-picker"
import { cn } from "@/lib/utils"
import { usePhone } from "@/logs/logs-results"
import { TopRowActions } from "@/shell/top-row-actions"

import type { SavedSearch } from "./queries"
import { SavedSearchItems } from "./saved-search-items"
import { SavedSearchesMenu } from "./saved-searches-menu"

type Props = {
  kind: SavedSearch["kind"]
  range: TimeRangeValue
  onRangeChange: (next: TimeRangeValue) => void
  tail?: { on: boolean; blocked: string | undefined; toggle: () => void }
  saveBlocked: string | undefined
  describedBy?: string
  saveTrigger: RefObject<HTMLButtonElement | null>
  onSave: () => void
  shareHref?: string
}

async function copyLink(href: string) {
  try {
    await navigator.clipboard.writeText(href)
    toast.success("Link copied")
  } catch {
    toast.error("Could not copy. Select the text and press Ctrl+C or Cmd+C.")
  }
}

export function ExploreActions({
  kind,
  range,
  onRangeChange,
  tail,
  saveBlocked,
  describedBy,
  saveTrigger,
  onSave,
  shareHref = window.location.href,
}: Props) {
  const phone = usePhone()
  return (
    <TopRowActions>
      <div className="flex items-center gap-2">
        <span className={cn("contents", phone && "[&_button]:h-11")}>
          <TimeRangeControl value={range} onChange={onRangeChange} />
        </span>
        {tail === undefined ? null : (
          <Button
            type="button"
            variant="outline"
            aria-pressed={tail.on}
            disabled={tail.blocked !== undefined}
            title={tail.blocked}
            aria-describedby={describedBy}
            className={cn(phone && "h-11")}
            onClick={tail.toggle}
          >
            <RadioIcon aria-hidden="true" />
            {tail.on ? "Stop live tail" : "Live tail"}
          </Button>
        )}
        {phone ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  ref={saveTrigger}
                  variant="ghost"
                  size="icon"
                  aria-label="More actions"
                  className="size-11"
                />
              }
            >
              <MoreHorizontalIcon aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuItem
                disabled={saveBlocked !== undefined}
                title={saveBlocked}
                aria-describedby={describedBy}
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
                  <SavedSearchItems kind={kind} />
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuItem
                className="min-h-11"
                onClick={() => void copyLink(shareHref)}
              >
                <LinkIcon aria-hidden="true" />
                Copy link
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <>
            <span aria-hidden="true" className="h-5 w-px bg-border" />
            <SavedSearchesMenu kind={kind} />
            <Button
              ref={saveTrigger}
              type="button"
              variant="ghost"
              disabled={saveBlocked !== undefined}
              title={saveBlocked}
              aria-describedby={describedBy}
              onClick={onSave}
            >
              <BookmarkIcon aria-hidden="true" />
              <span className="@max-[780px]/top:sr-only">Save search</span>
            </Button>
            <CopyButton
              text={shareHref}
              label="Copy link"
              variant="ghost"
              size="icon"
            />
          </>
        )}
      </div>
    </TopRowActions>
  )
}
