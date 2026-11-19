import { createFileRoute } from "@tanstack/react-router"

import { NotFoundPage } from "@/shell/not-found-page"

export const Route = createFileRoute("/_authed/_shell/$")({
  staticData: { crumbs: [{ label: "Page not found" }] },
  component: NotFoundPage,
})
