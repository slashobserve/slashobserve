import { useEffect, useId, useState, type RefObject } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { exploreKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { loadProblemText } from "@/service-accounts/queries"
import { SidePanel } from "@/shell/side-panel"
import { allPanels, isDataPanel, panelTitle, type DataPanel } from "@/spec"

import { addProblemText, newPanelBody } from "./block-rows"
import { useAddBlock, type Observation } from "./queries"

type Props = {
  observation: Observation
  verb: "Add" | "Suggest"
  onPicked: (blockId: string) => void
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

type DashboardChoice = { id: string; title: string; folder: string }
type PanelChoice = { id: string; title: string; type: string; panel: DataPanel }

const searchDelayMs = 250

function useDebounced(value: string): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])
  return debounced
}

function panelChoices(raw: unknown): PanelChoice[] {
  return allPanels(asDashboardSpec(raw))
    .map((entry) => entry.panel)
    .filter(isDataPanel)
    .map((panel) => ({
      id: panel.id,
      title: panelTitle(panel),
      type: panel.type,
      panel,
    }))
}

export function PanelPicker({
  observation,
  verb,
  onPicked,
  onClose,
  trigger,
}: Props) {
  const ids = useId()
  const queryClient = useQueryClient()
  const addBlock = useAddBlock(observation.id)
  const [searchText, setSearchText] = useState("")
  const [dashboard, setDashboard] = useState<DashboardChoice | null>(null)
  const [panel, setPanel] = useState<PanelChoice | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const search = useDebounced(searchText.trim())

  const dashboards = useQuery({
    queryKey: exploreKeys.targets(search),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { q: search || undefined, limit: 50 } },
          signal,
        })
        .then(unwrap),
    select: (data): DashboardChoice[] =>
      data.dashboards.map((entry) => ({
        id: entry.id,
        title: entry.title,
        folder: entry.folder.title,
      })),
    staleTime: 15_000,
  })
  const spec = useQuery({
    ...latestDashboardQuery(queryClient, dashboard?.id ?? ""),
    enabled: dashboard !== null,
    select: (body) => panelChoices(body.version.spec),
  })

  function submit() {
    if (panel === null || addBlock.isPending) return
    setProblem(null)
    addBlock.mutate(
      { kind: "panel", body: newPanelBody(panel.panel, observation) },
      {
        onSuccess: ({ block }) => {
          onPicked(block.id)
          onClose()
        },
        onError: (error) => setProblem(addProblemText(error)),
      }
    )
  }

  let panelField
  if (dashboard === null) {
    panelField = null
  } else if (spec.isError) {
    panelField = (
      <Alert variant="destructive">
        <AlertDescription>{loadProblemText(spec.error)}</AlertDescription>
      </Alert>
    )
  } else if (spec.data === undefined) {
    panelField = <Skeleton aria-busy="true" className="h-8" />
  } else if (spec.data.length === 0) {
    panelField = (
      <p className="text-[13px] text-muted-foreground">
        This dashboard has no data panels.
      </p>
    )
  } else {
    panelField = (
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-panel`}>Panel</Label>
        <Combobox<PanelChoice>
          items={spec.data}
          value={panel}
          disabled={addBlock.isPending}
          itemToStringLabel={(item) => item.title}
          isItemEqualToValue={(item, value) => item.id === value.id}
          onValueChange={setPanel}
        >
          <ComboboxInput
            id={`${ids}-panel`}
            disabled={addBlock.isPending}
            placeholder="Search panels"
          />
          <ComboboxContent>
            <ComboboxEmpty>No panels found</ComboboxEmpty>
            <ComboboxList>
              {(item: PanelChoice) => (
                <ComboboxItem key={item.id} value={item}>
                  <span className="min-w-0 flex-1 truncate">{item.title}</span>
                  <span className="flex-none text-xs text-muted-foreground">
                    {item.type}
                  </span>
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
    )
  }

  return (
    <SidePanel
      title={`${verb} a panel`}
      description="The panel stays live on this page, with the observation's time range when it has one."
      trigger={trigger}
      closeDisabled={addBlock.isPending}
      onClose={onClose}
      footer={
        <>
          <Button
            variant="outline"
            disabled={addBlock.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            focusableWhenDisabled
            disabled={panel === null || addBlock.isPending}
            aria-busy={addBlock.isPending}
            onClick={submit}
          >
            {verb}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-dashboard`}>Dashboard</Label>
        <Combobox<DashboardChoice>
          items={dashboards.data ?? []}
          value={dashboard}
          filter={null}
          disabled={addBlock.isPending}
          itemToStringLabel={(item) => item.title}
          isItemEqualToValue={(item, value) => item.id === value.id}
          onInputValueChange={setSearchText}
          onValueChange={(next) => {
            setDashboard(next)
            setPanel(null)
          }}
        >
          <ComboboxInput
            id={`${ids}-dashboard`}
            disabled={addBlock.isPending}
            placeholder="Search dashboards"
          />
          <ComboboxContent>
            <ComboboxEmpty>
              {dashboards.isError
                ? loadProblemText(dashboards.error)
                : dashboards.isPending
                  ? "Loading dashboards"
                  : "No dashboards found"}
            </ComboboxEmpty>
            <ComboboxList>
              {(item: DashboardChoice) => (
                <ComboboxItem key={item.id} value={item}>
                  <span className="min-w-0 flex-1 truncate">{item.title}</span>
                  <span className="max-w-[160px] flex-none truncate text-xs text-muted-foreground">
                    {item.folder}
                  </span>
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      </div>
      {panelField}
      {problem === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      )}
    </SidePanel>
  )
}
