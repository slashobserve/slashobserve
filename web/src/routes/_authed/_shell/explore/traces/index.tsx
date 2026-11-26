import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateTraceEntrySearch } from "@/traces/search"

export const Route = createFileRoute("/_authed/_shell/explore/traces/")({
  validateSearch: validateTraceEntrySearch,
  beforeLoad: ({ search }) => {
    throw redirect({ to: "/traces", search, replace: true })
  },
})
