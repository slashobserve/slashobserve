import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateLogsEntrySearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/explore/logs/")({
  validateSearch: validateLogsEntrySearch,
  beforeLoad: ({ search }) => {
    throw redirect({ to: "/logs", search, replace: true })
  },
})
