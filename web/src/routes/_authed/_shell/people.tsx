import { createFileRoute } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { useDocumentTitle } from "@/lib/document-title"

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
    </div>
  )
}
