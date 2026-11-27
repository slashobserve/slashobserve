import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { CircleAlertIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { foldersQuery } from "@/folders/queries"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { isEditableTarget } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { dashboardLink } from "@/lib/links"

import { DatasourcesCard } from "./datasources-card"
import { importFailure, previewFailure, specFailure } from "./errors"
import { FolderCard } from "./folder-card"
import {
  automaticFolder,
  defaultFolder,
  folderChoices,
  readRememberedFolder,
  requestedFolder,
} from "./folder-choice"
import type { HandedImport } from "./handoff"
import {
  newSource,
  previewQuery,
  useCreateDashboardFromSpec,
  useImportDashboard,
  type Mapping,
  type Source,
} from "./queries"
import {
  MAX_FILE_BYTES,
  MAX_IMPORT_BYTES,
  TOO_LARGE_MESSAGE,
  UNREADABLE_MESSAGE,
  exceedsImportLimit,
  requestBytes,
  summaryLine,
} from "./read-source"
import { ReportCard } from "./report-card"
import { blockingReason } from "./report"
import { SourceCard } from "./source-card"
import { readSpecSource, specSummaryLine } from "./spec-source"

const noMapping: Mapping = {}

function hasFiles(event: DragEvent): boolean {
  return event.dataTransfer?.types.includes("Files") === true
}

export function ImportPage({ initial }: { initial: HandedImport | null }) {
  const statusId = useId()
  const [source, setSource] = useState<Source | null>(() =>
    initial === null
      ? null
      : newSource(initial.name, initial.text, initial.pasted)
  )
  const [mapping, setMapping] = useState<Mapping>(noMapping)
  const [refusal, setRefusal] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const [pickedFolder, setPickedFolder] = useState<string | null>(null)
  const folders = useQuery(foldersQuery())
  const choices = useMemo(
    () => folderChoices(folders.data ?? []),
    [folders.data]
  )
  const handedFolder = folders.data?.find(
    (candidate) => candidate.id === initial?.folder
  )?.title
  const [remembered] = useState(readRememberedFolder)
  const folder =
    pickedFolder ?? handedFolder ?? defaultFolder(remembered, choices)
  const importDashboard = useImportDashboard()
  const createFromSpec = useCreateDashboardFromSpec()
  const resetImportMutation = importDashboard.reset
  const resetSpecMutation = createFromSpec.reset
  const resetImport = useCallback(() => {
    resetImportMutation()
    resetSpecMutation()
  }, [resetImportMutation, resetSpecMutation])
  const result = importDashboard.data ?? null
  const created = result ?? createFromSpec.data ?? null
  const specSource = useMemo(
    () => (source ? readSpecSource(source.text) : null),
    [source]
  )
  const saving = importDashboard.isPending || createFromSpec.isPending
  const savingNow = useRef(saving)
  useEffect(() => {
    savingNow.current = saving
  })

  const preview = useQuery(previewQuery(specSource ? null : source, mapping))
  const previewError = preview.isError ? previewFailure(preview.error) : null
  const previewData = preview.data
  const stale = preview.isPlaceholderData

  const replaceSource = useCallback(
    (next: Source | null, problem: string | null) => {
      if (savingNow.current) return
      resetImport()
      setSource(next)
      setMapping(noMapping)
      setRefusal(problem)
    },
    [resetImport]
  )

  const startOver = useCallback(
    () => replaceSource(null, null),
    [replaceSource]
  )

  const refuse = useCallback(
    (message = TOO_LARGE_MESSAGE) => replaceSource(null, message),
    [replaceSource]
  )

  const accept = useCallback(
    (name: string, text: string, pasted: boolean) => {
      if (exceedsImportLimit(text)) {
        refuse()
        return
      }
      replaceSource(newSource(name, text, pasted), null)
    },
    [refuse, replaceSource]
  )

  const acceptFile = useCallback(
    async (file: File) => {
      if (file.size > MAX_FILE_BYTES) {
        refuse()
        return
      }
      let text: string
      try {
        text = await file.text()
      } catch {
        refuse(UNREADABLE_MESSAGE)
        return
      }
      accept(file.name, text, false)
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

  const runImport = (update?: string) => {
    if (!source) return
    const title = requestedFolder(folder, choices)
    if (specSource) {
      createFromSpec.mutate({
        spec: specSource.spec,
        folderId: folders.data?.find((candidate) => candidate.title === title)
          ?.id,
      })
      return
    }
    importDashboard.mutate({
      source,
      mapping,
      folder: requestedFolder(folder, choices),
      update,
    })
  }

  const importError = importDashboard.isError
    ? importFailure(importDashboard.error)
    : createFromSpec.isError
      ? specFailure(createFromSpec.error)
      : null
  const datasources =
    result?.import.datasources ??
    importError?.datasources ??
    previewData?.datasources
  const report = result?.import.report ?? previewData?.report
  const sourceError = refusal
    ? { message: refusal, retry: false }
    : previewError

  const notReadyReason =
    previewData && !previewData.ready
      ? (blockingReason(previewData.datasources) ??
        "Some datasources are not configured.")
      : null
  const existing = specSource ? null : (previewData?.existing ?? null)
  const importDisabled = specSource
    ? created !== null || saving
    : !previewData || notReadyReason !== null || stale || saving

  let status
  if (created) {
    status = (
      <div
        id={statusId}
        role="status"
        className="flex flex-col gap-1 text-[13px]"
      >
        <p>
          <StatusLabel tone="ok">
            {specSource ? "Created" : "Imported"} as {created.dashboard.title},
            version {created.version.version}
          </StatusLabel>
        </p>
        <p>In folder {created.dashboard.folder.title}.</p>
        {result?.folder_note ? <p>{result.folder_note}</p> : null}
        {result?.warning ? <p>{result.warning}</p> : null}
      </div>
    )
  } else if (importError || notReadyReason || existing) {
    status = (
      <div className="flex flex-col gap-1">
        {importError ? (
          <p role="alert" className="flex items-start gap-2 text-[13px]">
            <CircleAlertIcon
              aria-hidden="true"
              className="mt-0.5 size-3.5 flex-none text-status-critical"
            />
            {importError.message}
          </p>
        ) : null}
        {notReadyReason ? (
          <p id={statusId} className="text-xs text-muted-foreground">
            {notReadyReason}
          </p>
        ) : existing ? (
          <p className="text-[13px]">
            Already imported as{" "}
            <Link
              {...dashboardLink(existing.dashboard_id, existing.title)}
              className="font-medium underline"
            >
              {existing.title}
            </Link>{" "}
            (version {existing.version}).{" "}
            {existing.can_update
              ? "Import as new makes a second copy; update it to save this file as its next version."
              : "You cannot edit it, so this import makes a second copy."}
          </p>
        ) : null}
      </div>
    )
  }

  const actions = created ? (
    <>
      <Button variant="outline" className="font-normal" onClick={startOver}>
        Import another
      </Button>
      <Link
        {...dashboardLink(created.dashboard.id, created.dashboard.title)}
        className={cn(buttonVariants())}
      >
        Open dashboard
      </Link>
    </>
  ) : (
    <>
      <Link
        to="/"
        search={{ tab: "import" }}
        className={cn(buttonVariants({ variant: "outline" }), "font-normal")}
      >
        Cancel
      </Link>
      {existing?.can_update ? (
        <Button
          variant="outline"
          className="font-normal"
          disabled={importDisabled}
          onClick={() => runImport(existing.dashboard_id)}
        >
          Update {existing.title}
        </Button>
      ) : null}
      <Button
        disabled={importDisabled}
        aria-busy={saving || undefined}
        aria-describedby={notReadyReason ? statusId : undefined}
        onClick={() => runImport()}
      >
        {saving
          ? specSource
            ? "Creating..."
            : "Importing..."
          : specSource
            ? "Create dashboard"
            : "Import as new dashboard"}
      </Button>
    </>
  )

  return (
    <div className="flex w-full flex-col gap-4">
      <PageHeader
        title={specSource ? "Import a dashboard" : "Import from Grafana"}
        description={
          specSource
            ? "A dashboard exported from SlashObserve is created from its Spec as it is."
            : "Every panel is kept; what cannot render yet becomes a placeholder listed in the report."
        }
      />
      <SourceCard
        source={source}
        hint={
          specSource
            ? specSummaryLine(specSource)
            : previewData
              ? summaryLine(previewData.summary)
              : previewError
                ? ""
                : "Reading the dashboard..."
        }
        dragging={dragging && !saving}
        error={sourceError}
        onFile={(file) => void acceptFile(file)}
        onText={acceptText}
        onRetry={() => void preview.refetch()}
      />
      {source ? (
        <>
          <FolderCard
            choices={choices}
            value={requestedFolder(folder, choices) ?? automaticFolder}
            failed={folders.isError}
            disabled={created !== null || saving}
            onChange={setPickedFolder}
          />
          {datasources ? (
            <DatasourcesCard
              datasources={datasources}
              mapping={mapping}
              onPick={pick}
              disabled={stale || created !== null || saving}
              busy={stale}
            />
          ) : null}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            {status ? <div className="min-w-0 flex-1">{status}</div> : null}
            <div className="ml-auto flex flex-wrap items-center gap-2">
              {actions}
            </div>
          </div>
          {report ? (
            <ReportCard
              key={result ? "imported" : source.id}
              report={report}
              imported={result !== null}
              busy={stale}
            />
          ) : null}
        </>
      ) : null}
    </div>
  )
}
