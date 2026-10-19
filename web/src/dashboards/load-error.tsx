import { LoadProblem } from "@/components/load-problem"
import { loadProblemText } from "@/service-accounts/queries"
import { ThemeSwitch } from "@/shell/theme-switch"

import { DashboardTopRow } from "./dashboard-top-row"

type Props = {
  title: string
  error: unknown
  retrying: boolean
  onRetry: () => void
}

export function DashboardLoadError({ title, error, retrying, onRetry }: Props) {
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <DashboardTopRow
        title="Dashboard"
        madeBy={null}
        versionControl={null}
        controls={[]}
        editing={null}
        actions={[<ThemeSwitch key="theme" />]}
      />
      <main className="mx-auto w-full max-w-lg p-6">
        <LoadProblem
          title={title}
          detail={loadProblemText(error)}
          retrying={retrying}
          onRetry={onRetry}
        />
      </main>
    </div>
  )
}
