import { DownloadIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { auditExportUrl, startDownload, type ExportFormat } from "./export"

export function useAuditExport(window: AuditWindowKey | null) {
  return {
    start(format: ExportFormat) {
      if (window !== null) startDownload(auditExportUrl(window, format))
    },
  }
}

export function ExportMenu({
  disabled,
  onExport,
}: {
  disabled: boolean
  onExport: (format: ExportFormat) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" disabled={disabled} focusableWhenDisabled />
        }
      >
        <DownloadIcon aria-hidden="true" />
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
