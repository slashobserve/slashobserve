import { createFileRoute, Link } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { PeopleTable } from "@/people/people-table"
import { RoleLegend } from "@/people/role-legend"
import { SignInBanner } from "@/people/sign-in-banner"

export const Route = createFileRoute("/_authed/_shell/people")({
  staticData: { crumbs: [{ label: "People" }] },
  component: PeoplePage,
})

function PeoplePage() {
  useDocumentTitle("People - SlashObserve")
  return (
    <RoleGate min="admin">
      <PageHeader
        title="People"
        description="Everyone who has signed in. Access comes from your identity provider, not from invites."
      />
      <div className="mb-4">
        <SignInBanner
          action={
            <Link
              to="/settings"
              hash="sign-in"
              className={cn(
                buttonVariants({ variant: "outline" }),
                "font-normal"
              )}
            >
              Sign-in settings
            </Link>
          }
        />
      </div>
      <RoleLegend />
      <PeopleTable />
    </RoleGate>
  )
}
