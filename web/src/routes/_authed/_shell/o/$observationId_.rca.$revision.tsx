import { createFileRoute } from "@tanstack/react-router"

import { RcaRevisionPage } from "@/rca/rca-revision-page"

export const Route = createFileRoute(
  "/_authed/_shell/o/$observationId_/rca/$revision"
)({
  staticData: {
    crumbs: [
      { label: "Observations", to: "/observations" },
      { label: "RCA revision" },
    ],
  },
  component: RevisionRoute,
})

function RevisionRoute() {
  const { observationId, revision } = Route.useParams()
  return (
    <RcaRevisionPage
      key={`${observationId}:${revision}`}
      observationId={observationId}
      revision={Number(revision)}
    />
  )
}
