import { Link } from "@tanstack/react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { dashboardLink, observationLink, proposalLink } from "@/lib/links"

import type { DatasourceError, HeldItem } from "./admin-queries"

const kindLabels: Record<HeldItem["kind"], string> = {
  dashboard: "Dashboard",
  observation: "Observation",
  saved_search: "Saved search",
  proposal: "Proposal",
}

function personalFolderCount(count: number): string {
  return count === 1 ? "1 in a personal folder" : `${count} in personal folders`
}

function ItemLink({ item }: { item: HeldItem }) {
  switch (item.kind) {
    case "dashboard":
      return <Link {...dashboardLink(item.id, item.title)}>{item.title}</Link>
    case "observation":
      return <Link {...observationLink(item.id)}>{item.title}</Link>
    case "proposal":
      return <Link {...proposalLink(item.id)}>{item.title}</Link>
    case "saved_search":
      return (
        <Link to="/f/$folderId" params={{ folderId: item.folder_id }}>
          {item.title}
        </Link>
      )
  }
}

export function RestrictedHolders({ error }: { error: DatasourceError }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>
          Dashboards, searches or observations that read this datasource are in
          folders other people can read. Move them to an Admins only folder, or
          remove the panels that read it, then save again.
        </p>
        <ul className="list-disc pl-4">
          {error.held.map((item) => (
            <li key={`${item.kind}-${item.id}`} className="wrap-anywhere">
              {kindLabels[item.kind]}:{" "}
              <span className="underline underline-offset-2">
                <ItemLink item={item} />
              </span>
              , in {item.folder_title}
            </li>
          ))}
          {error.unlisted > 0 ? <li>and {error.unlisted} more</li> : null}
          {error.hidden > 0 ? (
            <li>{personalFolderCount(error.hidden)}</li>
          ) : null}
        </ul>
      </AlertDescription>
    </Alert>
  )
}
