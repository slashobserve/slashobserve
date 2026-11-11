import { Link } from "@tanstack/react-router"

import type { FindingEnvelope } from "./queries"

const linkClass =
  "rounded-sm underline underline-offset-3 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"

export function FindingPlace({ envelope }: { envelope: FindingEnvelope }) {
  const { finding, folder } = envelope
  if (folder === null) return <>In no folder</>
  const link = (
    <Link
      to="/f/$folderId"
      params={{ folderId: folder.id }}
      className={linkClass}
    >
      {folder.personal ? "My Dashboards" : folder.title}
    </Link>
  )
  if (finding.dashboard_id !== null) {
    return <>On its dashboard&apos;s folder, {link}</>
  }
  return <>In {link}</>
}
