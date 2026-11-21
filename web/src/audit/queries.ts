import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable } from "@/api/errors"
import { auditKeys, type AuditWindowKey } from "@/api/keys"

import { unwrapAudit } from "./query-problem"

const pageSize = 100
const histogramBuckets = 60
export const facetValues = 8

const retryOnce = (count: number, error: unknown) =>
  count < 1 && isRetryable(error)

function windowQuery(window: AuditWindowKey) {
  return {
    q: window.q === "" ? undefined : window.q,
    since: window.since,
    until: window.until,
  }
}

export function auditEventsQuery(window: AuditWindowKey) {
  return infiniteQueryOptions({
    queryKey: auditKeys.events(window),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/audit/events", {
          params: {
            query:
              pageParam === undefined
                ? { ...windowQuery(window), limit: pageSize }
                : {
                    q: windowQuery(window).q,
                    cursor: pageParam,
                    limit: pageSize,
                  },
          },
          signal,
        })
        .then(unwrapAudit),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    retry: retryOnce,
    refetchOnWindowFocus: false,
  })
}

export function auditFacetsQuery(window: AuditWindowKey, field: string) {
  return queryOptions({
    queryKey: auditKeys.facets(window, field),
    queryFn: ({ signal }) =>
      api
        .GET("/audit/facets", {
          params: {
            query: { ...windowQuery(window), field, limit: facetValues },
          },
          signal,
        })
        .then(unwrapAudit),
    retry: retryOnce,
    refetchOnWindowFocus: false,
  })
}

export function auditHistogramQuery(window: AuditWindowKey) {
  return queryOptions({
    queryKey: auditKeys.histogram(window),
    queryFn: ({ signal }) =>
      api
        .GET("/audit/histogram", {
          params: {
            query: { ...windowQuery(window), buckets: histogramBuckets },
          },
          signal,
        })
        .then(unwrapAudit),
    retry: retryOnce,
    refetchOnWindowFocus: false,
  })
}

export function auditFieldsQuery() {
  return queryOptions({
    queryKey: auditKeys.fields(),
    queryFn: ({ signal }) =>
      api.GET("/audit/fields", { signal }).then(unwrapAudit),
    staleTime: Infinity,
  })
}

export function auditEventQuery(id: string) {
  return queryOptions({
    queryKey: auditKeys.event(id),
    queryFn: ({ signal }) =>
      api
        .GET("/audit/events/{id}", { params: { path: { id } }, signal })
        .then(unwrapAudit),
    retry: retryOnce,
  })
}
