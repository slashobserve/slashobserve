import { useQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { SignInBanner } from "@/people/sign-in-banner"
import { domainList, providerDisplayName } from "@/people/sign-in-policy"
import { loadProblemText } from "@/service-accounts/queries"

import { domainsText, sessionLength } from "./format"
import {
  EnvChip,
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "./settings-card"
import { SettingsSection } from "./settings-section"

function Pending() {
  return <Skeleton className="h-4 w-32" />
}

export function SignInSection() {
  const { principal } = useRouteContext({ from: "/_authed" })
  const isAdmin = roleAtLeast(principal.role, "admin")
  return (
    <SettingsSection
      title="Sign-in"
      description="Who can sign in, through your identity provider."
    >
      <SignInBanner
        action={
          isAdmin ? (
            <Link
              to="/people"
              className={cn(
                buttonVariants({ variant: "outline" }),
                "font-normal narrow:h-11"
              )}
            >
              Open People
            </Link>
          ) : null
        }
      />
      <SignInCard />
    </SettingsSection>
  )
}

function SignInCard() {
  const query = useQuery(instanceQuery())
  const hint = "Set once for the whole server."

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="Sign-in" hint={hint}>
        <SettingsProblem
          title="Could not load the sign-in settings"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }

  const instance = query.data
  return (
    <SettingsCard title="Sign-in" hint={hint}>
      <SettingsRow
        label="Provider name"
        note={<EnvChip name="SLASHOBSERVE_AUTH_PROVIDER_NAME" />}
      >
        {instance ? (
          (providerDisplayName(instance.oidc_provider_name) ?? "Not set")
        ) : (
          <Pending />
        )}
      </SettingsRow>
      <SettingsRow
        label="Identity provider"
        mono
        note={<EnvChip name="SLASHOBSERVE_OIDC_ISSUER_URL" />}
      >
        {instance ? instance.oidc_issuer_url : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Allowed email domains"
        mono={
          instance !== undefined &&
          !instance.any_email_domain &&
          domainList(instance.allowed_email_domains).length > 0
        }
        note={<EnvChip name="SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS" />}
      >
        {instance ? domainsText(instance) : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Session length"
        note={<EnvChip name="SLASHOBSERVE_SESSION_TTL" />}
      >
        {instance ? sessionLength(instance.session_ttl_seconds) : <Pending />}
      </SettingsRow>
    </SettingsCard>
  )
}
