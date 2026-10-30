import { useCallback, useEffect, useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { CircleAlertIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { foldersQuery } from "@/folders/queries"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { isEditableTarget } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"

import { DatasourcesCard } from "./datasources-card"
import { importFailure, previewFailure } from "./errors"
import { FolderCard } from "./folder-card"
import {
  automaticFolder,
  folderChoices,
  requestedFolder,
} from "./folder-choice"
import {
  newSource,
  previewQuery,
  useImportDashboard,
  type Mapping,
  type Source,
} from "./queries"
import {
  MAX_FILE_BYTES,
  MAX_IMPORT_BYTES,
  TOO_LARGE_MESSAGE,
  requestBytes,
  summaryLine,
} from "./read-source"
import { ReportCard } from "./report-card"
import { blockingReason } from "./report"
import { SourceCard } from "./source-card"

const noMapping: Mapping = {}

function hasFiles(event: DragEvent): boolean {
  return event.dataTransfer?.types.includes("Files") === true
}

export function ImportPage() {
  const statusId = useId()
  const [source, setSource] = useState<Source | null>(null)
  const [mapping, setMapping] = useState<Mapping>(noMapping)
  const [refusal, setRefusal] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const [folder, setFolder] = useState(automaticFolder)
  const folders = useQuery(foldersQuery())
  const choices = useMemo(
    () => folderChoices(folders.data ?? []),
    [folders.data]
  )
  const importDashboard = useImportDashboard()
  const resetImport = importDashboard.reset
  const result = importDashboard.data ?? null

  const preview = useQuery(previewQuery(source, mapping))
  const previewError = preview.isError ? previewFailure(preview.error) : null
  const previewData = preview.data
  const stale = preview.isPlaceholderData

  const startOver = useCallback(() => {
    resetImport()
    setSource(null)
    setMapping(noMapping)
    setRefusal(null)
  }, [resetImport])

  const refuse = useCallback(() => {
    resetImport()
    setSource(null)
    setMapping(noMapping)
    setRefusal(TOO_LARGE_MESSAGE)
  }, [resetImport])

  const accept = useCallback(
    (name: string, text: string, pasted: boolean) => {
      if (requestBytes(text, noMapping) > MAX_IMPORT_BYTES) {
        refuse()
        return
      }
      resetImport()
      setMapping(noMapping)
      setRefusal(null)
      setSource(newSource(name, text, pasted))
    },
    [refuse, resetImport]
  )

  const acceptFile = useCallback(
    async (file: File) => {
      if (file.size > MAX_FILE_BYTES) {
        refuse()
        return
      }
      accept(file.name, await file.text(), false)
    },
    [accept, refuse]
  )

  const acceptText = useCallback(
    (text: string) => accept("Pasted JSON", text, true),
    [accept]
  )

  useEffect(() => {
    let depth = 0
    const enter = (event: DragEvent) => {
      if (!hasFiles(event)) return
      depth += 1
      setDragging(true)
    }
    const leave = (event: DragEvent) => {
      if (!hasFiles(event)) return
      depth = Math.max(0, depth - 1)
      if (depth === 0) setDragging(false)
    }
    const over = (event: DragEvent) => {
      if (hasFiles(event)) event.preventDefault()
    }
    const drop = (event: DragEvent) => {
      if (!hasFiles(event)) return
      event.preventDefault()
      depth = 0
      setDragging(false)
      const file = event.dataTransfer?.files[0]
      if (file) void acceptFile(file)
    }
    const paste = (event: ClipboardEvent) => {
      if (isEditableTarget(event.target)) return
      const text = event.clipboardData?.getData("text/plain") ?? ""
      if (text.trim().startsWith("{")) acceptText(text)
    }
    window.addEventListener("dragenter", enter)
    window.addEventListener("dragleave", leave)
    window.addEventListener("dragover", over)
    window.addEventListener("drop", drop)
    window.addEventListener("paste", paste)
    return () => {
      window.removeEventListener("dragenter", enter)
      window.removeEventListener("dragleave", leave)
      window.removeEventListener("dragover", over)
      window.removeEventListener("drop", drop)
      window.removeEventListener("paste", paste)
    }
  }, [acceptFile, acceptText])

  if (previewError?.resetMapping && Object.keys(mapping).length > 0) {
    setMapping(noMapping)
  }

  const pick = (key: string, uid: string) => {
    if (!source) return
    const next = { ...mapping, [key]: uid }
    if (requestBytes(source.text, next) > MAX_IMPORT_BYTES) {
      setRefusal(TOO_LARGE_MESSAGE)
      return
    }
    setRefusal(null)
    resetImport()
    setMapping(next)
  }

  const runImport = () => {
    if (!source) return
    importDashboard.mutate({
      source,
      mapping,
      folder: requestedFolder(folder, choices),
    })
  }

  const importError = importDashboard.isError
    ? importFailure(importDashboard.error)
    : null
  const datasources =
    result?.import.datasources ??
    importError?.datasources ??
    previewData?.datasources
  const report = result?.import.report ?? previewData?.report
  const sourceError = refusal
    ? { message: refusal, retry: false }
    : previewError

  let statusLine: string | null = null
  if (!previewData) statusLine = "Choose a Grafana dashboard to import."
  else if (!previewData.ready) {
    statusLine =
      blockingReason(previewData.datasources) ??
      "Some datasources are not configured."
  }
  const importDisabled =
    statusLine !== null || stale || importDashboard.isPending

  let status
  if (result) {
    status = (
      <div
        id={statusId}
        role="status"
        className="flex flex-col gap-1 text-[13px]"
      >
        <p>
          <StatusLabel tone="ok">
            Imported as {result.dashboard.title}, version{" "}
            {result.version.version}
          </StatusLabel>
        </p>
        <p>In folder {result.dashboard.folder.title}.</p>
        {result.folder_note ? <p>{result.folder_note}</p> : null}
      </div>
    )
  } else if (importError) {
    status = (
      <p
        id={statusId}
        role="alert"
        className="flex items-start gap-2 text-[13px]"
      >
        <CircleAlertIcon
          aria-hidden="true"
          className="mt-0.5 size-3.5 flex-none text-status-critical"
        />
        {importError.message}
      </p>
    )
  } else if (statusLine) {
    status = (
      <p id={statusId} className="text-xs text-muted-foreground">
        {statusLine}
      </p>
    )
  }

  const actions = result ? (
    <>
      <Button variant="outline" className="font-normal" onClick={startOver}>
        Import another
      </Button>
      <Link
        to="/dashboards/$dashboardId"
        params={{ dashboardId: result.dashboard.id }}
        className={cn(buttonVariants())}
      >
        Open dashboard
      </Link>
    </>
  ) : (
    <>
      <Link
        to="/"
        className={cn(buttonVariants({ variant: "outline" }), "font-normal")}
      >
        Cancel
      </Link>
      <Button
        disabled={importDisabled}
        aria-busy={importDashboard.isPending || undefined}
        aria-describedby={statusLine ? statusId : undefined}
        onClick={runImport}
      >
        {importDashboard.isPending ? "Importing..." : "Import as new dashboard"}
      </Button>
    </>
  )

  return (
    <>
      <PageHeader
        title="Import from Grafana"
        description="Every panel is kept. What we cannot render yet becomes a placeholder and is listed in the report."
        actions={actions}
      />
      {status ? <div className="-mt-2.5 mb-4">{status}</div> : null}
      <div
        className="grid items-start gap-4"
        style={{
          gridTemplateColumns:
            "repeat(auto-fit, minmax(min(380px, 100%), 1fr))",
        }}
      >
        <div className="flex flex-col gap-4">
          <SourceCard
            source={source}
            hint={
              previewData
                ? summaryLine(previewData.summary)
                : previewError
                  ? ""
                  : "Reading the dashboard..."
            }
            dragging={dragging}
            error={sourceError}
            onFile={(file) => void acceptFile(file)}
            onText={acceptText}
            onRetry={() => void preview.refetch()}
          />
          {source ? (
            <FolderCard
              choices={choices}
              value={requestedFolder(folder, choices) ?? automaticFolder}
              failed={folders.isError}
              disabled={result !== null || importDashboard.isPending}
              onChange={setFolder}
            />
          ) : null}
          {datasources ? (
            <DatasourcesCard
              datasources={datasources}
              mapping={mapping}
              onPick={pick}
              disabled={stale || result !== null || importDashboard.isPending}
              busy={stale}
            />
          ) : null}
        </div>
        {report && source ? (
          <ReportCard
            key={result ? "imported" : source.id}
            report={report}
            imported={result !== null}
            busy={stale}
          />
        ) : null}
      </div>
    </>
  )
}
