import { EllipsisIcon, LinkIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"
import { cn } from "@/lib/utils"

import type { ResolvedLink } from "../field-config"
import { ActionMenuItems } from "./action-menu-items"
import type { FieldAction } from "./field-actions"

export function LinksMenu({
  links,
  actions,
  onAction,
  reason,
  subject,
  className,
  linkIcon = false,
}: {
  links: ResolvedLink[]
  actions: FieldAction[]
  onAction: (action: FieldAction) => void
  reason: string | null
  subject?: string
  className?: string
  linkIcon?: boolean
}) {
  const label = actions.length > 0 ? "More links and actions" : "More links"
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className={cn("text-muted-foreground", className)}
            aria-label={
              subject === undefined ? label : `${label} for ${subject}`
            }
          />
        }
      >
        {linkIcon && actions.length === 0 ? (
          <LinkIcon aria-hidden="true" />
        ) : (
          <EllipsisIcon aria-hidden="true" />
        )}
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
