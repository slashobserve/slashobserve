import type { ResolvedLink } from "@/panels/field-config"
import type { DashboardLink } from "@/spec"

import type { DashboardSearchParams } from "./search"

const maxTagsPerRequest = 10

export const tagLinkLimit = 100

type Listed = { id: string; title: string; tags: string[] }

export type DashboardsLink = DashboardLink & { type: "dashboards" }

export function isDashboardsLink(link: DashboardLink): link is DashboardsLink {
  return link.type === "dashboards"
}

export function requestTags(link: DashboardsLink): string[] {
  return (link.tags ?? []).slice(0, maxTagsPerRequest)
}

export function linkedDashboards<T extends Listed>(
  link: DashboardsLink,
  listed: T[],
  currentId: string
): T[] {
  const tags = link.tags ?? []
  return listed
    .filter(
      (dashboard) =>
        dashboard.id !== currentId &&
        tags.every((tag) => dashboard.tags.includes(tag))
    )
    .sort((a, b) => a.title.localeCompare(b.title))
}

function dashboardHref(id: string, search: DashboardSearchParams): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(search)) {
    if (value === undefined) continue
    for (const item of Array.isArray(value) ? value : [value]) {
      query.append(key, String(item))
    }
  }
  const text = query.toString()
  const path = `/dashboards/${encodeURIComponent(id)}`
  return text === "" ? path : `${path}?${text}`
}

export function taggedLinks(
  link: DashboardsLink,
  listed: Listed[],
  currentId: string,
  search: DashboardSearchParams
): ResolvedLink[] {
  return linkedDashboards(link, listed, currentId).map((dashboard) => ({
    title: dashboard.title,
    href: dashboardHref(dashboard.id, search),
    newTab: link.newTab === true,
  }))
}
