import { EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import { ActionMenuItems } from "../../actions/action-menu-items"
import type { FieldAction } from "../../actions/field-actions"
import type { ResolvedLink } from "../../field-config"

export function MoreLinks({
  links,
  actions,
  onAction,
  reason,
}: {
  links: ResolvedLink[]
  actions: FieldAction[]
  onAction: (action: FieldAction) => void
  reason: string | null
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            aria-label={
              actions.length > 0 ? "More links and actions" : "More links"
            }
          />
        }
      >
        <EllipsisIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        {links.map((link, index) => (
          <DropdownMenuItem
            key={`${index}:${link.href}`}
            render={<a {...anchorProps(link)} />}
            className="justify-between gap-3"
          >
            {link.title}
            {link.newTab ? <NewTabMark /> : null}
          </DropdownMenuItem>
        ))}
        {links.length > 0 && actions.length > 0 ? (
          <DropdownMenuSeparator />
        ) : null}
        <ActionMenuItems
          actions={actions}
          reason={reason}
          onAction={onAction}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
