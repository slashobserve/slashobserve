import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { DownloadIcon, LoaderCircleIcon, XIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import {
  auditExportUrl,
  downloadStartedText,
  exportLimitText,
  startDownload,
  type ExportFormat,
} from "./export"
import { auditHistogramQuery } from "./queries"

export function useAuditExport(window: AuditWindowKey | null) {
  const queryClient = useQueryClient()
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)

  async function start(format: ExportFormat) {
    if (window === null || pending) return
    setPending(true)
    setNotice(null)
    const total = await queryClient
      .ensureQueryData(auditHistogramQuery(window))
      .then((histogram) => histogram.total)
      .catch(() => null)
    setPending(false)
    setNotice(total === null ? null : exportLimitText(total))
    startDownload(auditExportUrl(window, format))
    toast.success(downloadStartedText(format))
  }

  return { pending, notice, start, dismiss: () => setNotice(null) }
}

export function ExportMenu({
  disabled,
  pending,
  onExport,
}: {
  disabled: boolean
  pending: boolean
  onExport: (format: ExportFormat) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            disabled={disabled || pending}
            focusableWhenDisabled
            aria-busy={pending}
          />
        }
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : (
          <DownloadIcon aria-hidden="true" />
        )}
        Export
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="left"
        align="start"
        sideOffset={8}
        className="w-max max-w-[calc(100vw-2rem)] min-w-64"
      >
        <DropdownMenuItem onClick={() => onExport("ndjson")}>
          NDJSON, one event per line
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => onExport("csv")}>
          CSV, with data as JSON text
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function ExportNotice({
  notice,
  onDismiss,
}: {
  notice: string
  onDismiss: () => void
}) {
  return (
    <Alert role="alert">
      <AlertDescription className="flex items-start justify-between gap-3">
        <p>{notice}</p>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Dismiss"
          className="text-foreground"
          onClick={onDismiss}
        >
          <XIcon aria-hidden="true" />
        </Button>
      </AlertDescription>
    </Alert>
  )
}
