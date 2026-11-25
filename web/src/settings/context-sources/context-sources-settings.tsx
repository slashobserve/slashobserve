import { useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { SettingsPanel } from "@/settings/sections"
import { SettingsCard, SettingsProblem } from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"

import { contextProblemText } from "./problems"
import { contextSourcesQuery } from "./queries"
import {
  SourcePanel,
  contextSourcesHint,
  type SourcePanelMode,
} from "./source-panel"
import { SourcesList } from "./sources-list"

type Props = {
  panel: SettingsPanel | null
  onPanel: (panel: SettingsPanel | null) => void
}

export function ContextSourcesSettings({ panel, onPanel }: Props) {
  const query = useQuery(contextSourcesQuery())
  const addButton = useRef<HTMLButtonElement>(null)
  const editing =
    panel?.kind === "context-edit"
      ? query.data?.find((source) => source.uid === panel.uid)
      : undefined
  const panelMode: SourcePanelMode | null =
    panel?.kind === "context-add"
      ? { kind: "add" }
      : editing === undefined
        ? null
        : { kind: "edit", source: editing }

  return (
    <SettingsSection
      title="Context sources"
      description="MCP servers the agent may read from. You decide which of their tools it can call."
      action={
        <Button
          ref={addButton}
          variant={panelMode === null ? "default" : "outline"}
          className="narrow:h-11"
          onClick={() => onPanel({ kind: "context-add" })}
        >
          <PlusIcon aria-hidden="true" />
          Add source
        </Button>
      }
    >
      <SettingsCard title="Context sources" hint={contextSourcesHint}>
        {query.isError && query.data === undefined ? (
          <SettingsProblem
            title="Could not load the context sources"
            detail={contextProblemText(query.error)}
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        ) : query.data === undefined ? (
          <div className="border-t p-4">
            <Skeleton className="h-16 w-full" />
          </div>
        ) : query.data.length === 0 ? (
          <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
            No context sources yet.
          </p>
        ) : (
          <div className="border-t">
            <SourcesList
              sources={query.data}
              editingUid={editing?.uid ?? null}
              onEdit={(uid) => onPanel({ kind: "context-edit", uid })}
            />
          </div>
        )}
      </SettingsCard>
      {panelMode === null ? null : (
        <SourcePanel
          mode={panelMode}
          onClose={() => onPanel(null)}
          trigger={addButton}
        />
      )}
    </SettingsSection>
  )
}
