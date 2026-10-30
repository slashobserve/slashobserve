import { EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { anchorProps, NewTabMark } from "@/dashboards/dashboard-links"

import type { ResolvedLink } from "../../field-config"

export function MoreLinks({ links }: { links: ResolvedLink[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="text-muted-foreground"
            aria-label="More links"
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
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
