import { useEffect, useId, useState, type RefObject } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { issueLines } from "@/api/issues"
import { exploreKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Skeleton } from "@/components/ui/skeleton"
import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "@/dashboards/queries"
import { dashboardLink } from "@/lib/links"
import { SidePanel } from "@/shell/side-panel"

import { appendLogsPanel } from "./add-panel"
import { exploreErrorText } from "./explore"

type Props = {
  datasource: string
  query: string
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
}

type Target = { id: string; title: string }

type Problem = { lines: string[]; hint?: string }

const searchDelayMs = 250
const skeletonRows = [0, 1, 2]
const saveRefusedHint =
  "A dashboard save runs every query over the dashboard's own time range, so a query with no lines in that range is refused."

function addProblem(error: unknown): Problem {
  if (error instanceof ApiError) {
    if (error.status === 409) {
      return {
        lines: [
          "Someone saved this dashboard a moment ago. Add again to place the panel on the latest version.",
        ],
      }
    }
    if (error.status === 422) {
      return {
        lines:
          error.issues.length > 0 ? issueLines(error.issues) : [error.message],
        hint: saveRefusedHint,
      }
    }
    if (error.status === 403)
      return { lines: ["You cannot edit this dashboard."] }
  }
  return { lines: [exploreErrorText(error)] }
}

function useDebounced(value: string): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])
  return debounced
}

function AddButton({
  target,
  title,
  datasource,
  query,
  onAdded,
  onProblem,
}: {
  target: Target
  title: string
  datasource: string
  query: string
  onAdded: () => void
  onProblem: (problem: Problem | null) => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const addVersion = useAddVersion(target.id)
  const [pending, setPending] = useState(false)

  async function add() {
    setPending(true)
    onProblem(null)
    try {
      const latest = await queryClient.fetchQuery({
        ...latestDashboardQuery(queryClient, target.id),
        staleTime: 0,
      })
      const spec = appendLogsPanel(asDashboardSpec(latest.version.spec), {
        title,
        datasource,
        query,
      })
      await addVersion.mutateAsync({
        spec,
        summary: "Add a logs panel from the logs page",
        parent_version: latest.version.version,
      })
      toast.success(`Added the panel to ${target.title}`, {
        action: {
          label: "Open dashboard",
          onClick: () => void navigate(dashboardLink(target.id, target.title)),
        },
      })
      onAdded()
    } catch (error) {
      onProblem(addProblem(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <Button
      type="button"
      focusableWhenDisabled
      disabled={pending || title.trim() === ""}
      aria-busy={pending}
      onClick={() => void add()}
    >
      Add panel
    </Button>
  )
}

export function AddToDashboardPanel({
  datasource,
  query,
  onClose,
  trigger,
}: Props) {
  const ids = useId()
  const [searchText, setSearchText] = useState("")
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [title, setTitle] = useState("Logs")
  const [problem, setProblem] = useState<Problem | null>(null)
  const search = useDebounced(searchText.trim())
  const targets = useQuery({
    queryKey: exploreKeys.targets(search),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { q: search || undefined, limit: 50 } },
          signal,
        })
        .then(unwrap),
    select: (data) =>
      data.dashboards
        .filter((dashboard) => dashboard.permissions.edit)
        .map((dashboard) => ({
          id: dashboard.id,
          title: dashboard.title,
          folder: dashboard.folder.title,
        })),
    staleTime: 15_000,
  })
  const selected = targets.data?.find((target) => target.id === selectedId)

  function list() {
    if (targets.isError) {
      return (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {exploreErrorText(targets.error)}
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => void targets.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )
    }
    if (targets.data === undefined) {
      return (
        <div aria-busy="true" className="flex flex-col gap-2">
          {skeletonRows.map((row) => (
            <Skeleton key={row} className="h-5" />
          ))}
        </div>
      )
    }
    if (targets.data.length === 0) {
      return (
        <p className="text-[13px] text-muted-foreground">
          No dashboard you can edit matches.
        </p>
      )
    }
    return (
      <RadioGroup
        aria-labelledby={`${ids}-targets`}
        value={selectedId ?? ""}
        onValueChange={(next) => setSelectedId(next as string)}
      >
        {targets.data.map((target) => (
          <label
            key={target.id}
            className="flex items-center gap-2 text-[13px]"
          >
            <RadioGroupItem value={target.id} />
            <span className="min-w-0 truncate">{target.title}</span>
            <span className="shrink-0 text-muted-foreground">
              {target.folder}
            </span>
          </label>
        ))}
      </RadioGroup>
    )
  }

  return (
    <SidePanel
      title="Add to dashboard"
      description="Adds a full-width logs panel with this query under the dashboard's last row."
      onClose={onClose}
      trigger={trigger}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          {selected ? (
            <AddButton
              key={selected.id}
              target={selected}
              title={title.trim()}
              datasource={datasource}
              query={query}
              onAdded={onClose}
              onProblem={setProblem}
            />
          ) : (
            <Button type="button" focusableWhenDisabled disabled>
              Add panel
            </Button>
          )}
        </>
      }
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-search`}>Search dashboards</Label>
        <Input
          id={`${ids}-search`}
          value={searchText}
          onChange={(event) => setSearchText(event.target.value)}
          autoComplete="off"
        />
      </div>
      <div className="flex flex-col gap-2">
        <p id={`${ids}-targets`} className="text-[13px] font-medium">
          Dashboards you can edit
        </p>
        {list()}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-title`}>Panel title</Label>
        <Input
          id={`${ids}-title`}
          value={title}
          maxLength={200}
          onChange={(event) => setTitle(event.target.value)}
        />
      </div>
      {problem ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col gap-1">
            {problem.lines.map((line) => (
              <p key={line}>{line}</p>
            ))}
            {problem.hint ? (
              <p className="text-muted-foreground">{problem.hint}</p>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
    </SidePanel>
  )
}
