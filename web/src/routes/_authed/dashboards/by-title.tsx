import { createFileRoute, redirect } from "@tanstack/react-router"

import {
  findByTitle,
  targetFor,
  validateByTitleSearch,
} from "@/dashboards/by-title"
import { dashboardLink } from "@/lib/links"

export const Route = createFileRoute("/_authed/dashboards/by-title")({
  validateSearch: validateByTitleSearch,
  beforeLoad: async ({ search }) => {
    const { title, ...carried } = search
    const candidates = title === undefined ? [] : await findByTitle(title)
    const target = targetFor(title, candidates)
    if (target.kind === "dashboard") {
      throw redirect({
        ...dashboardLink(target.id, title),
        search: carried,
        replace: true,
      })
    }
    throw redirect({
      to: "/dashboards",
      search: target.kind === "list" ? { q: target.title } : {},
      replace: true,
    })
  },
})
