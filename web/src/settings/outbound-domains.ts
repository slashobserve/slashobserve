import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { type ProblemTexts, unwrap } from "@/api/errors"
import { outboundDomainKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"

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
    mutationFn: (domain: string) =>
      api.POST("/outbound-domains", { body: { domain } }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: outboundDomainKeys.list() }),
  })
}

export function useRemoveDomain() {
  const queryClient = useQueryClient()
  return useMutation({
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

export const domainProblems: ProblemTexts = {
  codes: {
    domain_exists: "That domain is already allowed.",
    too_many_domains: "The list holds at most 100 domains. Remove one first.",
    managed_by_config:
      "This list is set by SLASHOBSERVE_OUTBOUND_DOMAINS on the server.",
  },
  notFound: "That domain was already removed.",
}

export function matchText(domain: string): string {
  return domain.startsWith("*.")
    ? `Subdomains of ${domain.slice(2)}`
    : `Only ${domain}`
}

const userKindText: Record<OutboundDomain["used_by"][number]["kind"], string> =
  {
    webhook: "Webhook",
    grafana_connection: "Grafana connection",
    datasource: "Datasource",
    mcp_source: "MCP source",
  }

function userText(user: OutboundDomain["used_by"][number]): string {
  return `${userKindText[user.kind]} ${user.name}`
}

export function removeConsequence(entry: OutboundDomain): string {
  const fetches =
    "Dashboard news feeds, actions and assets that fetch from it stop loading."
  if (entry.used_by.length === 0) return fetches
  return `${entry.used_by.map(userText).join(", ")} can no longer reach it. ${fetches}`
}

export function originText(entry: OutboundDomain): string | null {
  if (entry.added_at === null) return null
  if (entry.from_connection) return "Added for a connection or datasource"
  return entry.added_by === null ? "Added" : `Added by ${entry.added_by}`
}
