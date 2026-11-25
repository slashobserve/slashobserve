import { useEffect, useRef, useState } from "react"
import { PlusIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { McpSourcesSection } from "./context-sources/sources-table"
import type { DatasourceType } from "./draft"
import { DatasourcePanel } from "./datasource-panel"
import { DatasourceTable } from "./datasource-table"

type Target =
  { mode: "create"; type?: DatasourceType } | { mode: "edit"; uid: string }

type Props = { focusMcpSources?: boolean }

export function DatasourcesPage({ focusMcpSources = false }: Props) {
  const [target, setTarget] = useState<Target | null>(null)
  const [addingMcp, setAddingMcp] = useState(false)
  const trigger = useRef<HTMLElement | null>(null)
  const mcpHeading = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    if (!focusMcpSources) return
    mcpHeading.current?.scrollIntoView({ block: "start" })
    mcpHeading.current?.focus({ preventScroll: true })
  }, [focusMcpSources])

  function open(next: Target, from: HTMLElement) {
    trigger.current = from
    setTarget(next)
  }

  function pickMcp() {
    setTarget(null)
    setAddingMcp(true)
  }

  function pickNative(type: DatasourceType) {
    setAddingMcp(false)
    setTarget({ mode: "create", type })
  }

  function addMcp(from: HTMLElement) {
    trigger.current = from
    setAddingMcp(true)
  }

  return (
    <>
      <PageHeader
        title="Datasources"
        description="Where dashboards and the agent read data from. Entries from the datasources file and the built-in audit trail are read only; the ones you add here can be changed."
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
              Add source
            </Button>
          </div>
        }
      />
      <section aria-labelledby="datasources-heading">
        <h2 id="datasources-heading" className="mb-3 text-base font-semibold">
          Datasources
        </h2>
        <DatasourceTable
          onAdd={(from) => open({ mode: "create" }, from)}
          onEdit={(uid, from) => open({ mode: "edit", uid }, from)}
        />
        <p className="mt-3 text-xs text-muted-foreground">
          Addresses and users are shown only to Admins, and passwords, tokens
          and header values never leave the server.
        </p>
      </section>
      <div className="mt-10">
        <McpSourcesSection
          headingRef={mcpHeading}
          adding={addingMcp}
          addTrigger={trigger}
          onAddMcp={addMcp}
          onAddClose={() => setAddingMcp(false)}
          onPickNative={pickNative}
        />
      </div>
      {target === null ? null : (
        <DatasourcePanel
          key={target.mode === "edit" ? target.uid : "create"}
          target={target}
          trigger={trigger}
          onClose={() => setTarget(null)}
          onPickMcp={pickMcp}
        />
      )}
    </>
  )
}
