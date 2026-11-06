import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { publicKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import type { Dashboard } from "@/spec"

type PublicDashboardBody = components["schemas"]["PublicDashboard"]

export type PublicDashboard = Omit<PublicDashboardBody, "spec"> & {
  spec: Dashboard
}

function withoutQueryText(body: PublicDashboardBody): PublicDashboard {
  return { ...body, spec: body.spec as unknown as Dashboard }
}

export function publicDashboardQuery(token: string) {
  return queryOptions({
    queryKey: publicKeys.dashboard(token),
    queryFn: ({ signal }) =>
      api
        .GET("/public/{token}", { params: { path: { token } }, signal })
        .then(unwrap)
        .then(withoutQueryText),
    staleTime: 60_000,
  })
}
