import { ImageIcon, PrinterIcon } from "lucide-react"

import { DropdownMenuItem } from "@/components/ui/dropdown-menu"

import { useExportStore } from "./export-store"

export function ExportItems({ blockedBy }: { blockedBy: string | null }) {
  const start = useExportStore((state) => state.start)
  return (
    <>
      <DropdownMenuItem
        disabled={blockedBy !== null}
        onClick={() => start("print")}
      >
        <PrinterIcon aria-hidden="true" />
        {blockedBy === null
          ? "Print or save as PDF"
          : `Print or save as PDF (${blockedBy})`}
      </DropdownMenuItem>
      <DropdownMenuItem
        disabled={blockedBy !== null}
        onClick={() => start("image")}
      >
        <ImageIcon aria-hidden="true" />
        {blockedBy === null
          ? "Download as image"
          : `Download as image (${blockedBy})`}
      </DropdownMenuItem>
    </>
  )
}
