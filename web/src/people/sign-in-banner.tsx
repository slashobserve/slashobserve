import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { StatusLabel } from "@/components/status-label"
import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import {
  domainList,
  domainSeparator,
  isAllowAll,
  providerLabel,
} from "./sign-in-policy"

export function SignInBanner() {
  const query = useQuery(instanceQuery())
  if (!query.data) return null

  const provider = providerLabel(query.data.oidc_provider_name)
  const allowAll = isAllowAll(query.data)
  const domains = domainList(query.data.allowed_email_domains)
  if (!allowAll && domains.length === 0) return null

  return (
    <div className="mb-4 flex flex-wrap items-center gap-2.5 rounded-card border px-3.5 py-3 text-[13px]">
      <StatusLabel tone={allowAll ? "warning" : "ok"}>{null}</StatusLabel>
      <span className="min-w-60 flex-1">
        {allowAll ? (
          <>Anyone with a verified email can sign in through {provider}.</>
        ) : (
          <>
            Anyone with a verified email at{" "}
            {domains.map((domain, index) => (
              <span key={domain}>
                {domainSeparator(index, domains.length)}
                <span className="font-mono text-xs">{domain}</span>
              </span>
            ))}{" "}
            can sign in through {provider}.
          </>
        )}
      </span>
      <Link
        to="/settings"
        className={cn(buttonVariants({ variant: "outline" }), "font-normal")}
      >
        Sign-in settings
      </Link>
    </div>
  )
}
