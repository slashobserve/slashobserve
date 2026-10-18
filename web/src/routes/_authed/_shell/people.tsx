import { createFileRoute } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { useDocumentTitle } from "@/lib/document-title"
import { PeopleTable } from "@/people/people-table"
import { SignInBanner } from "@/people/sign-in-banner"

export const Route = createFileRoute("/_authed/_shell/people")({
  staticData: { crumbs: [{ label: "People" }] },
  component: PeoplePage,
})

function PeoplePage() {
  useDocumentTitle("People - SlashObserve")
  return (
    <div className="mx-auto w-full max-w-5xl">
      <PageHeader
        title="People"
        description="Everyone who has signed in. Access comes from your identity provider, not from invites."
      />
      <SignInBanner />
      <PeopleTable />
    </div>
  )
}
