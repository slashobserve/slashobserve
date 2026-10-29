import { ExternalLinkIcon } from "lucide-react"

import { useNow } from "@/components/relative-time"
import {
  dashboardLinks,
  type LinkScope,
  type ResolvedLink,
} from "@/panels/field-config"
import type { Link } from "@/spec"

export function anchorProps(link: ResolvedLink) {
  return {
    href: link.href,
    target: link.newTab ? "_blank" : undefined,
    rel: link.newTab ? "noopener noreferrer" : undefined,
  }
}

export function NewTabMark() {
  return (
    <>
      <ExternalLinkIcon aria-hidden="true" className="size-3" />
      <span className="sr-only">(opens in new tab)</span>
    </>
  )
}

export function DashboardLinks({
  links,
  scope,
}: {
  links: Link[] | undefined
  scope: LinkScope
}) {
  const now = useNow()
  const resolved = dashboardLinks(links, scope, now)
  if (resolved.length === 0) return null
  return (
    <nav
      aria-label="Dashboard links"
      className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 pb-2"
    >
      {resolved.map((link, index) => (
        <a
          key={`${index}:${link.href}`}
          {...anchorProps(link)}
          className="inline-flex items-center gap-1 text-[13px] text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
        >
          {link.title}
          {link.newTab ? <NewTabMark /> : null}
        </a>
      ))}
    </nav>
  )
}
