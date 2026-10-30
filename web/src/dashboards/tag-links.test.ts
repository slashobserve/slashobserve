import { describe, expect, it } from "vitest"

import {
  isDashboardsLink,
  type DashboardsLink,
  linkedDashboards,
  requestTags,
  taggedLinks,
} from "./tag-links"

const link = (tags?: string[]): DashboardsLink => ({
  type: "dashboards",
  title: "Related",
  tags,
})

const listed = [
  { id: "1", title: "Pods", tags: ["k8s", "prod"] },
  { id: "2", title: "Nodes", tags: ["k8s"] },
  { id: "3", title: "API", tags: ["k8s", "prod", "edge"] },
  { id: "4", title: "Billing", tags: [] },
]

describe("linkedDashboards", () => {
  it("keeps only the dashboards that carry every tag, by title", () => {
    expect(
      linkedDashboards(link(["k8s", "prod"]), listed, "none").map(
        (dashboard) => dashboard.id
      )
    ).toEqual(["3", "1"])
  })

  it("leaves out the dashboard it is shown on", () => {
    expect(
      linkedDashboards(link(["k8s"]), listed, "1").map(
        (dashboard) => dashboard.id
      )
    ).toEqual(["3", "2"])
  })

  it("lists every other dashboard when the link has no tags", () => {
    expect(linkedDashboards(link(), listed, "4")).toHaveLength(3)
    expect(linkedDashboards(link([]), listed, "4")).toHaveLength(3)
  })
})

describe("requestTags", () => {
  it("asks the server for at most the 10 tags it accepts", () => {
    const tags = Array.from({ length: 14 }, (_, i) => `t${i}`)
    expect(requestTags(link(tags))).toEqual(tags.slice(0, 10))
    expect(requestTags(link())).toEqual([])
  })
})

describe("isDashboardsLink", () => {
  it("tells a link to dashboards from a link to an address", () => {
    expect(isDashboardsLink(link())).toBe(true)
    expect(isDashboardsLink({ title: "Docs", url: "/docs" })).toBe(false)
  })
})

describe("taggedLinks", () => {
  it("points at each dashboard with the carried range and selections", () => {
    expect(
      taggedLinks(link(["prod"]), listed, "1", {
        from: "now-1h",
        to: "now",
        "var-pod": ["a", "b"],
      })
    ).toEqual([
      {
        title: "API",
        href: "/dashboards/3?from=now-1h&to=now&var-pod=a&var-pod=b",
        newTab: false,
      },
    ])
  })

  it("opens in a new tab on request", () => {
    const [first] = taggedLinks(
      { ...link(["prod"]), newTab: true },
      listed,
      "3",
      {}
    )
    expect(first).toEqual({
      title: "Pods",
      href: "/dashboards/1",
      newTab: true,
    })
  })
})
