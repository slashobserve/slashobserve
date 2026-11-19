import { useRef, useState } from "react"
import { PlusIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { DatasourcePanel } from "./datasource-panel"
import { DatasourceTable } from "./datasource-table"

type Target = { mode: "create" } | { mode: "edit"; uid: string }

export function DatasourcesPage() {
  const [target, setTarget] = useState<Target | null>(null)
  const trigger = useRef<HTMLElement | null>(null)

  function open(next: Target, from: HTMLElement) {
    trigger.current = from
    setTarget(next)
  }

  return (
    <>
      <PageHeader
        title="Datasources"
        description="Where dashboards and the agent read data from. Entries from the datasources file are read only; the ones you add here can be changed."
        actions={
          <div className="flex flex-wrap gap-2">
            <a
              href="https://github.com/slashobserve/slashobserve/blob/main/docs/connect/datasources-in-the-app.md"
              target="_blank"
              rel="noopener noreferrer"
              className={cn(
                buttonVariants({ variant: "outline" }),
                "font-normal"
              )}
            >
              How datasources work
              <span className="sr-only"> (opens in a new tab)</span>
            </a>
            <Button
              onClick={(event) => open({ mode: "create" }, event.currentTarget)}
            >
              <PlusIcon aria-hidden="true" />
              Add datasource
            </Button>
          </div>
        }
      />
      <DatasourceTable
        onAdd={(from) => open({ mode: "create" }, from)}
        onEdit={(uid, from) => open({ mode: "edit", uid }, from)}
      />
      <p className="mt-3 text-xs text-muted-foreground">
        Addresses and users are shown only to Admins, and passwords, tokens and
        header values never leave the server.
      </p>
      {target === null ? null : (
        <DatasourcePanel
          key={target.mode === "edit" ? target.uid : "create"}
          target={target}
          trigger={trigger}
          onClose={() => setTarget(null)}
        />
      )}
    </>
  )
}
