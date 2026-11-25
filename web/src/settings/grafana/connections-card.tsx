import { useRef, useState } from "react"
import { EllipsisIcon, LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Alert, AlertDescription } from "@/components/ui/alert"
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { grafanaProblemText } from "./connection-problems"
import {
  useDeleteConnection,
  useTestConnection,
  type GrafanaConnection,
} from "./queries"
import { summaryText } from "./results"

function connectionId(connection: GrafanaConnection): string {
  return connection.id
}

type Props = {
  connections: GrafanaConnection[]
  onImport: (connection: GrafanaConnection) => void
  onEdit: (
    connection: GrafanaConnection,
    trigger: HTMLButtonElement | undefined
  ) => void
}

function LastTest({ connection }: { connection: GrafanaConnection }) {
  const test = connection.last_test
  if (test === null) {
    return <span className="text-muted-foreground">Not tested</span>
  }
  return (
    <div className="flex flex-col gap-0.5">
      <StatusLabel tone={test.ok ? "ok" : "critical"}>
        {test.message}
      </StatusLabel>
      <span className="text-xs text-muted-foreground">
        <RelativeTime iso={test.at} />
      </span>
    </div>
  )
}

function LastImport({ connection }: { connection: GrafanaConnection }) {
  const sync = connection.last_sync
  if (sync === null) {
    return <span className="text-muted-foreground">Never</span>
  }
  return (
    <div className="flex flex-col gap-0.5">
      <span>{summaryText(sync)}</span>
      <span className="text-xs text-muted-foreground">
        <RelativeTime iso={sync.finished_at} />
      </span>
    </div>
  )
}

export function ConnectionsCard({ connections, onImport, onEdit }: Props) {
  const test = useTestConnection()
  const remove = useDeleteConnection()
  const [deleting, setDeleting] = useState<GrafanaConnection | null>(null)
  const deleteButtons = useRef(new Map<string, HTMLButtonElement>())
  const deleteTarget = useRef<string | null>(null)

  function runTest(connection: GrafanaConnection) {
    test.mutate(connection.id, {
      onError: (error) => toast.error(grafanaProblemText(error)),
    })
  }

  function changeDeleteOpen(open: boolean) {
    if (open || remove.isPending) return
    setDeleting(null)
    remove.reset()
  }

  function confirmDelete() {
    if (deleting === null) return
    const name = deleting.name
    remove.mutateAsync(deleting.id).then(
      () => {
        setDeleting(null)
        toast.success(`Deleted ${name}`)
      },
      () => {}
    )
  }

  const columns: DataGridColumn<GrafanaConnection>[] = [
    {
      id: "name",
      header: "Name",
      minWidth: 140,
      autoHeight: true,
      cell: (connection) => (
        <span className="py-2 font-medium wrap-anywhere">
          {connection.name}
        </span>
      ),
    },
    {
      id: "url",
      header: "URL",
      minWidth: 200,
      flex: 1.5,
      autoHeight: true,
      cell: (connection) => (
        <span className="py-2 font-mono text-xs wrap-anywhere">
          {connection.url}
        </span>
      ),
    },
    {
      id: "lastTest",
      header: "Last test",
      minWidth: 140,
      autoHeight: true,
      cell: (connection) => (
        <div className="py-2">
          <LastTest connection={connection} />
        </div>
      ),
    },
    {
      id: "lastImport",
      header: "Last import",
      minWidth: 140,
      autoHeight: true,
      cell: (connection) => (
        <div className="py-2">
          <LastImport connection={connection} />
        </div>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 250,
      cell: (connection) => {
        const testing = test.isPending && test.variables === connection.id
        return (
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              className="narrow:h-11"
              disabled={testing}
              focusableWhenDisabled
              aria-busy={testing}
              aria-label={`Test ${connection.name}`}
              onClick={() => runTest(connection)}
            >
              {testing ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Test
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="narrow:h-11"
              aria-label={`Import from ${connection.name}`}
              onClick={() => onImport(connection)}
            >
              Import
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    ref={(button) => {
                      if (button === null)
                        deleteButtons.current.delete(connection.id)
                      else deleteButtons.current.set(connection.id, button)
                    }}
                    variant="outline"
                    size="icon-sm"
                    className="narrow:size-11"
                    aria-label={`More for ${connection.name}`}
                  />
                }
              >
                <EllipsisIcon aria-hidden="true" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  onClick={() =>
                    onEdit(connection, deleteButtons.current.get(connection.id))
                  }
                >
                  Edit
                </DropdownMenuItem>
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => {
                    deleteTarget.current = connection.id
                    setDeleting(connection)
                  }}
                >
                  Delete
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
    },
  ]

  return (
    <>
      <DataGrid
        label="Grafana connections"
        rows={connections}
        columns={columns}
        rowId={connectionId}
        bare
        className="border-t"
      />
      <AlertDialog open={deleting !== null} onOpenChange={changeDeleteOpen}>
        <AlertDialogContent
          finalFocus={() =>
            deleteButtons.current.get(deleteTarget.current ?? "") ?? true
          }
        >
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Delete {deleting?.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              The stored URL and token are removed, and so is its host on the
              Outbound access list unless a webhook, another connection or an
              entry added by hand uses it. Dashboards imported from this Grafana
              stay, with their folders and import records.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {remove.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {grafanaProblemText(remove.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={confirmDelete}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete connection
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
