import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { instanceKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

export type Instance =
  paths["/instance"]["get"]["responses"][200]["content"]["application/json"]

export function instanceQuery() {
  return queryOptions({
    queryKey: instanceKeys.info(),
    queryFn: ({ signal }) => api.GET("/instance", { signal }).then(unwrap),
    staleTime: Infinity,
  })
}
