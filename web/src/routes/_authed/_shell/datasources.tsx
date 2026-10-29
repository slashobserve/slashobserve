import { createFileRoute } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { DatasourceTable } from "@/datasources/datasource-table"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/_authed/_shell/datasources")({
  staticData: { crumbs: [{ label: "Datasources" }] },
  component: DatasourcesPage,
})

function DatasourcesPage() {
  useDocumentTitle("Datasources - SlashObserve")
  return (
    <RoleGate min="admin">
      <PageHeader
        title="Datasources"
        description="Defined in the datasources file on the server. Edit that file and restart to add or change one."
        actions={
          <a
            href="https://github.com/slashobserve/slashobserve/blob/main/docs/reference/datasources-file.md"
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              buttonVariants({ variant: "outline" }),
              "font-normal"
            )}
          >
            How to add a datasource
            <span className="sr-only"> (opens in a new tab)</span>
          </a>
        }
      />
      <DatasourceTable />
      <p className="mt-3 text-xs text-muted-foreground">
        Addresses, users and secrets stay on the server and never reach the
        browser.
      </p>
    </RoleGate>
  )
}
