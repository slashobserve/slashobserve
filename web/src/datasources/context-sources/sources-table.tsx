import { useMemo, useRef, useState, type Ref } from "react"
import { useQuery } from "@tanstack/react-query"
import { ChevronRightIcon, LoaderCircleIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { LoadProblem } from "@/components/load-problem"
import { StatusLabel } from "@/components/status-label"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { audienceLabels, contextProblemText, sourceStatus } from "./problems"
import {
  contextSourcesQuery,
  useDeleteContextSource,
  useDiscoverContextSource,
  type ContextSource,
} from "./queries"
import { SourcePanel, type SourcePanelMode } from "./source-panel"
import { ToolsList } from "./tools-list"

const mcpSourcesCaption = "MCP servers the agent may read from"

function sourceId(source: ContextSource): string {
  return source.uid
}

type Props = {
  headingRef?: Ref<HTMLHeadingElement>
}

export function McpSourcesSection({ headingRef }: Props) {
  const query = useQuery(contextSourcesQuery())
  const addButton = useRef<HTMLElement | null>(null)
  const [panel, setPanel] = useState<SourcePanelMode | null>(null)
  const [openUids, setOpenUids] = useState<readonly string[]>([])
  const [deleting, setDeleting] = useState<ContextSource | null>(null)
  const remove = useDeleteContextSource()
  const openDetail = useMemo(() => new Set(openUids), [openUids])

  function setOpen(uid: string, open: boolean) {
    setOpenUids((current) =>
      open
        ? current.includes(uid)
          ? current
          : [...current, uid]
        : current.filter((id) => id !== uid)
    )
  }

  function confirmDelete() {
    if (deleting === null) return
    remove.mutateAsync(deleting.uid).then(
      () => {
        toast.success(`Deleted ${deleting.name}`)
        setOpen(deleting.uid, false)
        setDeleting(null)
      },
      () => {}
    )
  }

  const columns: DataGridColumn<ContextSource>[] = [
    {
      id: "name",
      header: "Name",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: (source) => (
        <SourceName
          source={source}
          open={openDetail.has(source.uid)}
          onToggle={() => setOpen(source.uid, !openDetail.has(source.uid))}
        />
      ),
    },
    {
      id: "url",
      header: "URL",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: (source) => (
        <div className="flex min-w-0 flex-col py-2">
          <span className="font-mono text-xs wrap-anywhere">{source.url}</span>
          {source.provisioned ? (
            <span className="text-xs text-muted-foreground">
              Set by the context sources file
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "audience",
      header: "Who may use it",
      minWidth: 120,
      autoHeight: true,
      cell: (source) => (
        <div className="flex flex-col py-2">
          <span>{audienceLabels[source.audience]}</span>
          {source.secret_set ? (
            <span className="text-xs text-muted-foreground">Secret stored</span>
          ) : null}
        </div>
      ),
    },
    {
      id: "health",
      header: "Health",
      minWidth: 170,
      autoHeight: true,
      cell: (source) => {
        const status = sourceStatus(source)
        return (
          <div className="flex flex-col gap-0.5 py-2">
            <div className="flex">
              <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
            </div>
            {source.state === "unreachable" ? (
              <span className="pl-[19px] text-xs text-muted-foreground">
                {status.detail}
              </span>
            ) : null}
          </div>
        )
      },
    },
    {
      id: "approved",
      header: "Approved",
      minWidth: 90,
      cell: (source) => (
        <span className="text-muted-foreground">{source.tools.approved}</span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      minWidth: 230,
      flex: 1,
      autoHeight: true,
      cell: (source) => (
        <div className="flex flex-wrap justify-end gap-1.5 py-1.5">
          <DiscoverButton
            source={source}
            onFound={() => setOpen(source.uid, true)}
          />
          <Button
            size="xs"
            variant="outline"
            disabled={source.provisioned}
            aria-label={`Edit ${source.name}`}
            onClick={(event) => {
              addButton.current = event.currentTarget
              setPanel({ kind: "edit", source })
            }}
          >
            Edit
          </Button>
          <Button
            size="xs"
            variant="outline"
            disabled={source.provisioned}
            aria-label={`Delete ${source.name}`}
            onClick={() => setDeleting(source)}
          >
            Delete
          </Button>
        </div>
      ),
    },
  ]

  return (
    <section aria-labelledby="mcp-sources-heading" id="mcp-sources">
      <div className="mb-3 flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        <div>
          <h2
            id="mcp-sources-heading"
            ref={headingRef}
            tabIndex={-1}
            className="text-base font-semibold outline-none"
          >
            MCP sources
          </h2>
          <p className="text-[13px] text-muted-foreground">
            {mcpSourcesCaption}
          </p>
        </div>
        <Button
          variant={panel === null ? "default" : "outline"}
          onClick={(event) => {
            addButton.current = event.currentTarget
            setPanel({ kind: "add" })
          }}
        >
          <PlusIcon aria-hidden="true" />
          Add source
        </Button>
      </div>
      {query.isError && query.data === undefined ? (
        <LoadProblem
          title="Could not load the MCP sources"
          detail={contextProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      ) : query.data === undefined ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          {[0, 1].map((index) => (
            <Skeleton key={index} className="h-5 w-full" />
          ))}
        </div>
      ) : query.data.length === 0 ? (
        <div className="rounded-card border px-4 py-10 text-center">
          <h3 className="text-sm font-semibold">No MCP sources yet</h3>
          <p className="text-[13px] text-muted-foreground">
            Register an MCP server, discover its tools and approve the ones the
            agent may call.
          </p>
        </div>
      ) : (
        <DataGrid
          label="MCP sources"
          rows={query.data}
          columns={columns}
          rowId={sourceId}
          rowAttributes={(source) => ({ "data-uid": source.uid })}
          highlightedRowId={panel?.kind === "edit" ? panel.source.uid : null}
          detail={{
            open: openDetail,
            render: (source) => (
              <div id={`context-tools-${source.uid}`}>
                <ToolsList uid={source.uid} />
              </div>
            ),
          }}
        />
      )}
      {panel === null ? null : (
        <SourcePanel
          mode={panel}
          onClose={() => setPanel(null)}
          trigger={addButton}
        />
      )}
      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open || remove.isPending) return
          setDeleting(null)
          remove.reset()
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleting?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              The source, its approved tools and its stored secret are removed.
              The agent can no longer use its tools. Its domain stays on the
              outbound allowlist unless nothing else uses it.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {remove.isError ? (
            <p role="alert" className="text-[13px] text-destructive">
              {contextProblemText(remove.error)}
            </p>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              onClick={(event) => {
                event.preventDefault()
                confirmDelete()
              }}
            >
              Delete source
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

function SourceName({
  source,
  open,
  onToggle,
}: {
  source: ContextSource
  open: boolean
  onToggle: () => void
}) {
  return (
    <div className="flex min-w-0 items-start gap-1.5 py-1.5">
      <Button
        variant="ghost"
        size="icon-xs"
        aria-expanded={open}
        aria-controls={`context-tools-${source.uid}`}
        aria-label={`Tools of ${source.name}`}
        onClick={onToggle}
      >
        <ChevronRightIcon
          aria-hidden="true"
          className={cn(
            "motion-safe:transition-transform",
            open && "rotate-90"
          )}
        />
      </Button>
      <div className="flex min-w-0 flex-col">
        <span className="min-w-0 font-medium wrap-break-word">
          {source.name}
        </span>
        <span className="font-mono text-xs font-normal wrap-anywhere text-muted-foreground">
          {source.uid}
        </span>
        {source.description === null ? null : (
          <span className="mt-0.5 text-[13px] wrap-anywhere">
            {source.description}
          </span>
        )}
      </div>
    </div>
  )
}

function DiscoverButton({
  source,
  onFound,
}: {
  source: ContextSource
  onFound: () => void
}) {
  const discover = useDiscoverContextSource()

  function run() {
    discover.mutate(source.uid, {
      onSuccess: ({ source: found, tally }) => {
        if (found.failure !== null) {
          toast.error(`${source.name}: ${sourceStatus(found).detail}`)
          return
        }
        onFound()
        const changed =
          tally.changed > 0 ? `, ${tally.changed} changed and stopped` : ""
        toast.success(
          `${source.name}: found ${tally.found} ${tally.found === 1 ? "tool" : "tools"}, ${tally.new} new${changed}`
        )
      },
      onError: (error) => toast.error(contextProblemText(error)),
    })
  }

  return (
    <Button
      size="xs"
      variant="outline"
      disabled={discover.isPending}
      aria-busy={discover.isPending}
      aria-label={`Discover the tools of ${source.name}`}
      onClick={run}
    >
      {discover.isPending ? (
        <LoaderCircleIcon
          aria-hidden="true"
          className="motion-safe:animate-spin"
        />
      ) : null}
      Discover
    </Button>
  )
}
