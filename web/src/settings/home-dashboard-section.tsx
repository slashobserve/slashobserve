import { Link } from "@tanstack/react-router"

import { loadProblemText } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useClearHomeDashboard, useHomeDashboard } from "@/home/home-dashboard"
import { dashboardLink } from "@/lib/links"

import { SettingsCard, SettingsProblem, SettingsRow } from "./settings-card"
import { SettingsSection } from "./settings-section"

const hint =
  "Opens after you sign in, in place of Home. Choose it from a dashboard's More menu."

const unusable: Record<string, string> = {
  gone: "The dashboard was deleted, so Home opens instead.",
  unreadable: "You can no longer view the dashboard, so Home opens instead.",
} as const

function HomeDashboardCard() {
  const query = useHomeDashboard()
  const clear = useClearHomeDashboard()
  const home = query.data
  if (query.isError && home === undefined) {
    return (
      <SettingsCard title="Opens after sign-in" hint={hint}>
        <SettingsProblem
          title="Could not load your home dashboard"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  if (home === undefined) {
    return (
      <SettingsCard title="Opens after sign-in" hint={hint}>
        <SettingsRow label="Dashboard">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }
  return (
    <SettingsCard
      title="Opens after sign-in"
      hint={hint}
      action={
        home.status === "none" ? null : (
          <Button
            variant="outline"
            size="sm"
            disabled={clear.isPending}
            onClick={() => clear.mutate()}
          >
            Clear
          </Button>
        )
      }
    >
      <SettingsRow label="Dashboard">
        {home.status === "none" ? (
          <span className="text-muted-foreground">
            None chosen, Home opens after sign-in.
          </span>
        ) : home.dashboard ? (
          <>
            <Link
              {...dashboardLink(home.dashboard.id, home.dashboard.title)}
              className="underline-offset-2 hover:underline"
            >
              {home.dashboard.title}
            </Link>
            <span className="text-muted-foreground">
              {" "}
              in {home.dashboard.folder_title}
            </span>
          </>
        ) : (
          <span className="text-muted-foreground">{unusable[home.status]}</span>
        )}
      </SettingsRow>
    </SettingsCard>
  )
}

export function HomeDashboardSection() {
  return (
    <SettingsSection
      title="Home dashboard"
      description="Pick a dashboard to open straight after you sign in, such as the one your team watches."
    >
      <HomeDashboardCard />
    </SettingsSection>
  )
}
