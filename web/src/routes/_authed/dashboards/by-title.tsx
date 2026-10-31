import { createFileRoute, redirect } from "@tanstack/react-router"

import {
  findByTitle,
  targetFor,
  validateByTitleSearch,
} from "@/dashboards/by-title"

export const Route = createFileRoute("/_authed/dashboards/by-title")({
  validateSearch: validateByTitleSearch,
  beforeLoad: async ({ search }) => {
    const { title, ...carried } = search
    const candidates = title === undefined ? [] : await findByTitle(title)
    const target = targetFor(title, candidates)
    if (target.kind === "dashboard") {
      throw redirect({
        to: "/dashboards/$dashboardId",
        params: { dashboardId: target.id },
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
