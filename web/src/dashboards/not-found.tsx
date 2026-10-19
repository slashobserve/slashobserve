import { Link } from "@tanstack/react-router"
import { KeyRoundIcon } from "lucide-react"

import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { ThemeSwitch } from "@/shell/theme-switch"
import { useSignOut } from "@/shell/use-sign-out"

import { DashboardTopRow } from "./dashboard-top-row"

export function DashboardNotFound() {
  const { signOut, pending } = useSignOut()
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <DashboardTopRow
        title="Not found"
        madeBy={null}
        versionControl={null}
        controls={[]}
        editing={null}
        actions={[
          <ThemeSwitch key="theme" />,
          <Link
            key="keys"
            to="/keys"
            className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
          >
            <KeyRoundIcon data-icon="inline-start" />
            API keys
          </Link>,
          <Button
            key="sign-out"
            variant="outline"
            size="sm"
            disabled={pending}
            aria-busy={pending}
            onClick={signOut}
          >
            Sign out
          </Button>,
        ]}
      />
      <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
        <h1 className="text-2xl font-semibold">Dashboard not found</h1>
        <p className="text-sm text-muted-foreground">
          It may have been removed, or the link is wrong.
        </p>
        <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
          Back to dashboards
        </Link>
      </main>
    </div>
  )
}
