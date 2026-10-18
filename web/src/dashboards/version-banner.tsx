import { useParams } from "@tanstack/react-router"

import { Alert, AlertAction, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { RestoreVersionButton } from "@/history/restore-version-button"

import { dashboardRouteId, useSetDashboardSearch } from "./search"

type Props = { version: number; latestVersion: number }

export function VersionBanner({ version, latestVersion }: Props) {
  const setSearch = useSetDashboardSearch()
  const { dashboardId } = useParams({ from: dashboardRouteId })
  const unpin = () => setSearch((state) => ({ ...state, version: undefined }))
  if (version >= latestVersion) return null
  return (
    <div className="px-4 pt-3">
      <Alert className="flex flex-wrap items-center justify-between gap-2 py-1.5 has-data-[slot=alert-action]:pr-1.5">
        <AlertDescription className="text-foreground">
          You are viewing version {version}. The latest is version{" "}
          {latestVersion}.
        </AlertDescription>
        <AlertAction className="static flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={unpin}>
            Back to latest
          </Button>
          <RestoreVersionButton
            variant="banner"
            dashboardId={dashboardId}
            version={version}
            latestVersion={latestVersion}
            onRestored={unpin}
          />
        </AlertAction>
      </Alert>
    </div>
  )
}
