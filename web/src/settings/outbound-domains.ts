import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { outboundDomainKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { loadProblemText } from "@/service-accounts/queries"

export type OutboundDomains =
  paths["/outbound-domains"]["get"]["responses"][200]["content"]["application/json"]

export type OutboundDomain = OutboundDomains["domains"][number]

export function outboundDomainsQuery() {
  return queryOptions({
    queryKey: outboundDomainKeys.list(),
    queryFn: ({ signal }) =>
      api.GET("/outbound-domains", { signal }).then(unwrap),
  })
}

export function useAllowDomain() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (domain: string) =>
      api.POST("/outbound-domains", { body: { domain } }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: outboundDomainKeys.list() }),
  })
}

export function useRemoveDomain() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (domain: string) =>
      api
        .DELETE("/outbound-domains/{domain}", {
          params: { path: { domain } },
        })
        .then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: outboundDomainKeys.list() }),
  })
}

export function domainDraftError(text: string): string | null {
  const draft = text.trim()
  if (draft === "") return null
  if (draft.includes("://")) return "Enter a domain, not a URL: example.com"
  if (/[/:@\s]/.test(draft)) {
    return "Enter only the domain name, like example.com or *.example.com."
  }
  return null
}

export function domainProblemText(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "domain_exists":
        return "That domain is already allowed."
      case "too_many_domains":
        return "The list holds at most 100 domains. Remove one first."
      case "managed_by_config":
        return "This list is set by SLASHOBSERVE_OUTBOUND_DOMAINS on the server."
      case "invalid_domain":
        return loadProblemText(error)
    }
    if (error.status === 404) return "That domain was already removed."
  }
  return loadProblemText(error)
}

export function matchText(domain: string): string {
  return domain.startsWith("*.")
    ? `Subdomains of ${domain.slice(2)}`
    : `Only ${domain}`
}
