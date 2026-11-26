import type { ReactNode, RefObject } from "react"
import { AtSignIcon, MessageSquarePlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { shortcutText } from "@/shortcuts/chords"
import { panelTitle, type LayoutPanel } from "@/spec"

import {
  askAboutThisPanel,
  commentOnThisPanel,
  useConversationActions,
} from "./panel-actions"
import { useSetDashboardSearch } from "./search"

const hoverFadeClass = cn(
  "[@media(hover:hover)_and_(pointer:fine)]:opacity-0",
  "motion-safe:transition-opacity",
  "motion-safe:duration-100",
  "motion-safe:ease-[var(--ease-out)]",
  "[@media(hover:hover)_and_(pointer:fine)]:group-hover/panel:opacity-100",
  "[@media(hover:hover)_and_(pointer:fine)]:group-focus-within/panel:opacity-100"
)

const overlayOnFinePointer = cn(
  "[@media(hover:hover)_and_(pointer:fine)]:absolute",
  "[@media(hover:hover)_and_(pointer:fine)]:inset-y-0",
  "[@media(hover:hover)_and_(pointer:fine)]:right-full",
  "[@media(hover:hover)_and_(pointer:fine)]:mr-0.5",
  "[@media(hover:hover)_and_(pointer:fine)]:bg-card",
  "[@media(hover:hover)_and_(pointer:fine)]:pointer-events-none",
  "[@media(hover:hover)_and_(pointer:fine)]:group-hover/panel:pointer-events-auto",
  "[@media(hover:hover)_and_(pointer:fine)]:group-focus-within/panel:pointer-events-auto",
  "[@media(hover:hover)_and_(pointer:fine)]:before:absolute",
  "[@media(hover:hover)_and_(pointer:fine)]:before:inset-y-0",
  "[@media(hover:hover)_and_(pointer:fine)]:before:right-full",
  "[@media(hover:hover)_and_(pointer:fine)]:before:w-3",
  "[@media(hover:hover)_and_(pointer:fine)]:before:bg-linear-to-r",
  "[@media(hover:hover)_and_(pointer:fine)]:before:from-transparent",
  "[@media(hover:hover)_and_(pointer:fine)]:before:to-card"
)

const hiddenWhileToolbarShown = "group-data-[selected]/panel:hidden"

type Props = {
  panel: LayoutPanel
  ownerId?: string
  canComment: boolean
  idle: boolean
  viewing?: boolean
  onCanvas: boolean
  composerRef: RefObject<HTMLTextAreaElement | null>
}

function HoverAction({
  label,
  tooltip,
  disabled,
  onClick,
  children,
}: {
  label: string
  tooltip: string
  disabled: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  )
}

export function PanelHoverActions({
  panel,
  ownerId = panel.id,
  canComment,
  idle,
  viewing = false,
  onCanvas,
  composerRef,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const actions = useConversationActions(panel, canComment)
  const title = panelTitle(panel)
  if (!actions.comment) return null
  const pause = idle ? "" : " (finish the layout first)"
  return (
    <div
      data-panel-hover-actions=""
      className={cn(
        "flex items-center gap-0.5 @max-[15rem]/panel:hidden",
        hoverFadeClass,
        overlayOnFinePointer,
        !viewing && hiddenWhileToolbarShown
      )}
    >
      {actions.ask ? (
        <HoverAction
          label={`Ask about ${title}`}
          tooltip={`Ask (${shortcutText("ask")})${pause}`}
          disabled={!idle}
          onClick={() => askAboutThisPanel(setSearch, ownerId, onCanvas)}
        >
          <AtSignIcon aria-hidden="true" />
        </HoverAction>
      ) : null}
      <HoverAction
        label={`Comment on ${title}`}
        tooltip={`Comment (${shortcutText("comment")})${pause}`}
        disabled={!idle}
        onClick={() =>
          commentOnThisPanel(setSearch, ownerId, onCanvas, composerRef)
        }
      >
        <MessageSquarePlusIcon aria-hidden="true" />
      </HoverAction>
    </div>
  )
}
