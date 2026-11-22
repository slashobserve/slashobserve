import { useId } from "react"

import { PageHeader } from "@/components/page-header"
import { FolderObservations } from "@/folders/folder-observations"

import { AuditFolderGate } from "./audit-folder-gate"

export function AuditObservations() {
  const headingId = useId()
  return (
    <AuditFolderGate title="Observations">
      {(folder) => (
        <>
          <PageHeader
            headingId={headingId}
            title="Observations"
            description={`In the ${folder.title} folder.`}
          />
          <FolderObservations
            key={folder.id}
            folderId={folder.id}
            labelledBy={headingId}
          />
        </>
      )}
    </AuditFolderGate>
  )
}
