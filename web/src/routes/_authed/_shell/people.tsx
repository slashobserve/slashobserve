import { createFileRoute, Link } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { InviteButton } from "@/people/invite-dialog"
import { InvitesTable } from "@/people/invites-table"
import { PeopleTable } from "@/people/people-table"
import { RoleLegend } from "@/people/role-legend"
import { SignInBanner } from "@/people/sign-in-banner"
import { TeamsCard } from "@/people/teams-card"

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
        description="Everyone who has signed in, and the people invited with a role for their first sign-in. Your identity provider still decides who can sign in."
        actions={<InviteButton />}
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
      <InvitesTable />
      <RoleLegend />
      <PeopleTable />
      <TeamsCard />
    </RoleGate>
  )
}
