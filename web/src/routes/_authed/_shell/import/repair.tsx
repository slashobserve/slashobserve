import { createFileRoute, redirect } from "@tanstack/react-router"

import { importedFilter } from "@/health/health-labels"

type RepairSearch = { folder?: string }

export const Route = createFileRoute("/_authed/_shell/import/repair")({
  validateSearch: (search: Record<string, unknown>): RepairSearch => ({
    folder:
      typeof search.folder === "string" && search.folder !== ""
        ? search.folder
        : undefined,
  }),
  beforeLoad: ({ search }) => {
    throw redirect({
      to: "/health",
      search: {
        kind: importedFilter,
        ...(search.folder === undefined ? {} : { folder: search.folder }),
      },
      replace: true,
    })
  },
})
