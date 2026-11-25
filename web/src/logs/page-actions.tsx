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
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { cn } from "@/lib/utils"
import { SavedSearchItems } from "@/saved-searches/saved-search-items"
import { SavedSearchesMenu } from "@/saved-searches/saved-searches-menu"
import type { logsDraft } from "@/saved-searches/draft"
import { TopRowActions } from "@/shell/top-row-actions"

import { usePhone } from "./logs-results"

type Savable = ReturnType<typeof logsDraft>

type Props = {
  range: { from: string; to: string }
  onRangeChange: (next: { from: string; to: string }) => void
  tail: { on: boolean; start: () => void; stop: () => void }
  queryRun: boolean
  notRunId: string
  savable: Savable
  saveTrigger: RefObject<HTMLButtonElement | null>
  onSave: () => void
}

const notRunText = "Run a query first."

function saveHint(savable: Savable): string | undefined {
  if (!("blocked" in savable)) return undefined
  return savable.blocked === "no-query"
    ? notRunText
    : "Filters are not applied: switch the query to KQL."
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(window.location.href)
    toast.success("Link copied")
  } catch {
    toast.error("Could not copy. Select the text and press Ctrl+C or Cmd+C.")
  }
}

function MoreActions({
  savable,
  saveTrigger,
  onSave,
  describedBy,
}: Pick<Props, "savable" | "saveTrigger" | "onSave"> & {
  describedBy: string | undefined
}) {
  const hint = saveHint(savable)
  return (
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
          disabled={"blocked" in savable}
          title={hint}
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
            <SavedSearchItems kind="logs" />
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

export function LogsPageActions({
  range,
  onRangeChange,
  tail,
  queryRun,
  notRunId,
  savable,
  saveTrigger,
  onSave,
}: Props) {
  const phone = usePhone()
  const describedBy = queryRun ? undefined : notRunId
  const liveTail = (
    <Button
      type="button"
      variant="outline"
      aria-pressed={tail.on}
      disabled={!queryRun}
      title={queryRun ? undefined : notRunText}
      aria-describedby={describedBy}
      className={cn(phone && "h-11")}
      onClick={() => (tail.on ? tail.stop() : tail.start())}
    >
      <RadioIcon aria-hidden="true" />
      {tail.on ? "Stop live tail" : "Live tail"}
    </Button>
  )
  const timeRange = (
    <span className={cn("contents", phone && "[&_button]:h-11")}>
      <TimeRangeControl value={range} onChange={onRangeChange} />
    </span>
  )

  if (phone) {
    return (
      <TopRowActions>
        <div className="flex items-center gap-2">
          {timeRange}
          {liveTail}
          <MoreActions
            savable={savable}
            saveTrigger={saveTrigger}
            onSave={onSave}
            describedBy={describedBy}
          />
        </div>
      </TopRowActions>
    )
  }
  return (
    <TopRowActions>
      <div className="flex items-center gap-2">
        {timeRange}
        {liveTail}
        <span aria-hidden="true" className="h-5 w-px bg-border" />
        <SavedSearchesMenu kind="logs" />
        <Button
          ref={saveTrigger}
          type="button"
          variant="ghost"
          disabled={"blocked" in savable}
          title={saveHint(savable)}
          aria-describedby={describedBy}
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
      </div>
    </TopRowActions>
  )
}
