import { useMemo, type MouseEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon, ExternalLinkIcon } from "lucide-react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { useNow } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  dashboardLinks,
  oneClickLink,
  type LinkScope,
  type ResolvedLink,
} from "@/panels/field-config"
import { dashlistSearch } from "@/panels/dashlist/dashlist-links"
import type { TimeOptions } from "@/lib/time"
import type { Dashboard } from "@/spec"

import {
  isDashboardsLink,
  requestTags,
  type DashboardsLink,
  tagLinkLimit,
  taggedLinks,
} from "./tag-links"

export function anchorProps(link: ResolvedLink) {
  return {
    href: link.href,
    target: link.newTab ? "_blank" : undefined,
    rel: link.newTab ? "noopener noreferrer" : undefined,
  }
}

const interactiveTarget = "a, button, input, [role='menuitem']"

function followOneClick(link: ResolvedLink, event: MouseEvent<HTMLElement>) {
  const { target } = event
  if (
    event.defaultPrevented ||
    !(target instanceof Element) ||
    target.closest(interactiveTarget) ||
    window.getSelection()?.toString()
  ) {
    return
  }
  if (link.newTab) window.open(link.href, "_blank", "noopener,noreferrer")
  else window.location.assign(link.href)
}

export function oneClickProps(links: ResolvedLink[]) {
  const link = oneClickLink(links)
  if (!link) return {}
  return {
    "data-one-click": "",
    onClick: (event: MouseEvent<HTMLElement>) => followOneClick(link, event),
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

const linkClass =
  "inline-flex items-center gap-1 text-[13px] text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"

function AnchorLink({ link }: { link: ResolvedLink }) {
  return (
    <a {...anchorProps(link)} className={linkClass}>
      {link.title}
      {link.newTab ? <NewTabMark /> : null}
    </a>
  )
}

type Scope = LinkScope & { from: string; to: string }

function TaggedLinks({
  link,
  dashboardId,
  scope,
}: {
  link: DashboardsLink
  dashboardId: string
  scope: Scope
}) {
  const tags = requestTags(link)
  const list = useQuery({
    queryKey: dashboardKeys.tagged(tags),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              tag: tags.length === 0 ? undefined : tags,
              limit: tagLinkLimit,
            },
          },
          signal,
        })
        .then(unwrap),
    staleTime: 15_000,
  })
  const { from, to, vars } = scope
  const search = useMemo(
    () =>
      dashlistSearch(
        {
          keepTime: link.keepTime === true,
          includeVars: link.includeVars === true,
        },
        { from, to },
        vars
      ),
    [link.keepTime, link.includeVars, from, to, vars]
  )
  const targets = useMemo(
    () => taggedLinks(link, list.data?.dashboards ?? [], dashboardId, search),
    [link, list.data, dashboardId, search]
  )
  if (targets.length === 0) return null
  if (!link.asDropdown) {
    return targets.map((target) => (
      <AnchorLink key={target.href} link={target} />
    ))
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1.5 text-[13px] font-normal text-muted-foreground"
          />
        }
      >
        {link.title}
        <ChevronDownIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-max">
        {targets.map((target) => (
          <DropdownMenuItem
            key={target.href}
            render={<a {...anchorProps(target)} />}
            className="justify-between gap-3"
          >
            {target.title}
            {target.newTab ? <NewTabMark /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function DashboardLinks({
  links,
  dashboardId,
  scope,
  timeOptions,
}: {
  links: Dashboard["links"]
  dashboardId: string
  scope: Scope
  timeOptions: TimeOptions
}) {
  const now = useNow()
  if (!links || links.length === 0) return null
  const items = links.flatMap((link, index) => {
    if (isDashboardsLink(link)) {
      return [
        <TaggedLinks
          key={`${index}:tags`}
          link={link}
          dashboardId={dashboardId}
          scope={scope}
        />,
      ]
    }
    return dashboardLinks([link], scope, now, timeOptions).map((resolved) => (
      <AnchorLink key={`${index}:${resolved.href}`} link={resolved} />
    ))
  })
  if (items.length === 0) return null
  return (
    <nav
      aria-label="Dashboard links"
      className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 pb-2"
    >
      {items}
    </nav>
  )
}
