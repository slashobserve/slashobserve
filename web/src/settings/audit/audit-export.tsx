import { useState } from "react"
import { DownloadIcon, LoaderCircleIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import type { AuditWindowKey } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { loadProblemText } from "@/service-accounts/queries"

import { downloadAuditExport, type ExportFormat } from "./export"

type Notice = { tone: "default" | "destructive"; text: string }

export function useAuditExport(window: AuditWindowKey | null) {
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)

  async function start(format: ExportFormat) {
    if (window === null || pending) return
    setPending(true)
    setNotice(null)
    try {
      const outcome = await downloadAuditExport(window, format)
      if (outcome.kind === "complete") {
        toast.success(
          `Exported ${outcome.rows.toLocaleString("en-US")} ${outcome.rows === 1 ? "event" : "events"}.`
        )
      } else {
        setNotice({
          tone: outcome.kind === "incomplete" ? "destructive" : "default",
          text: outcome.message,
        })
      }
    } catch (error) {
      setNotice({
        tone: "destructive",
        text: `The export did not start. ${loadProblemText(error)}`,
      })
    } finally {
      setPending(false)
    }
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
      <DropdownMenuContent align="end">
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
  notice: Notice
  onDismiss: () => void
}) {
  return (
    <Alert variant={notice.tone} role="alert">
      <AlertDescription className="flex items-start justify-between gap-3">
        <p>{notice.text}</p>
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
