import { useState } from "react"
import { ChevronDownIcon, LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

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
import { cn } from "@/lib/utils"

import {
  audienceLabels,
  contextProblemText,
  sourceStatus,
  toolSummary,
} from "./problems"
import {
  useDeleteContextSource,
  useDiscoverContextSource,
  type ContextSource,
} from "./queries"
import { ToolsList } from "./tools-list"

type Props = {
  sources: ContextSource[]
  editingUid: string | null
  onEdit: (uid: string) => void
}

function SourceRow({
  source,
  editing,
  onEdit,
  onDelete,
}: {
  source: ContextSource
  editing: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  const discover = useDiscoverContextSource()
  const [open, setOpen] = useState(false)
  const status = sourceStatus(source)
  const toolsId = `context-tools-${source.uid}`

  function runDiscover() {
    discover.mutate(source.uid, {
      onSuccess: ({ source: found, tally }) => {
        if (found.failure !== null) {
          toast.error(`${source.name}: ${sourceStatus(found).detail}`)
          return
        }
        setOpen(true)
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
    <li
      data-uid={source.uid}
      className={cn("border-t first:border-t-0", editing && "bg-muted/40")}
    >
      <div className="flex flex-wrap items-start gap-x-4 gap-y-2 px-4 py-3">
        <div className="min-w-[220px] flex-1">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="text-sm font-medium wrap-anywhere">
              {source.name}
            </span>
            <span className="font-mono text-xs text-muted-foreground">
              {source.uid}
            </span>
          </div>
          {source.description === null ? null : (
            <p className="mt-0.5 text-[13px] wrap-anywhere">
              {source.description}
            </p>
          )}
          <p className="mt-0.5 font-mono text-xs wrap-anywhere">{source.url}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {audienceLabels[source.audience]}
            {source.secret_set ? ", secret stored" : ""}
            {source.provisioned ? ", set by the context sources file" : ""}
          </p>
        </div>
        <div className="flex min-w-[180px] flex-col gap-0.5 text-[13px]">
          <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
          <span className="text-xs text-muted-foreground">{status.detail}</span>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            className="narrow:h-11"
            disabled={discover.isPending}
            aria-busy={discover.isPending}
            aria-label={`Discover the tools of ${source.name}`}
            onClick={runDiscover}
          >
            {discover.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Discover
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="narrow:h-11"
            aria-expanded={open}
            aria-controls={toolsId}
            aria-label={`Tools of ${source.name}: ${toolSummary(source.tools)}`}
            onClick={() => setOpen((current) => !current)}
          >
            Tools
            <ChevronDownIcon
              aria-hidden="true"
              className={cn(open && "rotate-180")}
            />
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="narrow:h-11"
            disabled={source.provisioned}
            aria-label={`Edit ${source.name}`}
            onClick={onEdit}
          >
            Edit
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="narrow:h-11"
            disabled={source.provisioned}
            aria-label={`Delete ${source.name}`}
            onClick={onDelete}
          >
            Delete
          </Button>
        </div>
      </div>
      {open ? (
        <div id={toolsId} className="border-t bg-background">
          <ToolsList uid={source.uid} />
        </div>
      ) : null}
    </li>
  )
}

export function SourcesList({ sources, editingUid, onEdit }: Props) {
  const remove = useDeleteContextSource()
  const [deleting, setDeleting] = useState<ContextSource | null>(null)

  function confirmDelete() {
    if (deleting === null) return
    remove.mutateAsync(deleting.uid).then(
      () => {
        toast.success(`Deleted ${deleting.name}`)
        setDeleting(null)
      },
      () => {}
    )
  }

  return (
    <>
      <ul>
        {sources.map((source) => (
          <SourceRow
            key={source.uid}
            source={source}
            editing={source.uid === editingUid}
            onEdit={() => onEdit(source.uid)}
            onDelete={() => setDeleting(source)}
          />
        ))}
      </ul>
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
    </>
  )
}
