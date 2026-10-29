import { createFileRoute } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { PageHeader } from "@/components/page-header"
import { useDocumentTitle } from "@/lib/document-title"
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
      <SignInBanner />
      <RoleLegend />
      <PeopleTable />
    </RoleGate>
  )
}
