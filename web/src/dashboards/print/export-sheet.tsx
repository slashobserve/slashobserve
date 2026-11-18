import { useEffect, useRef, useState, type ReactNode } from "react"
import { ArrowLeftIcon, PrinterIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"

import { holdSheet, useExportStore } from "./export-store"
import { stampedAt } from "./header-text"
import { saveDashboardImage } from "./image"
import { sheetWidth } from "./print-grid"
import { waitForPanels } from "./settle"
import "./print.css"

const settleTimeoutMs = 30_000
const settleMs = 400

function statusText(mode: string | null, ready: boolean): string {
  if (mode === "image") {
    return ready ? "Drawing the image." : "Loading every panel before drawing."
  }
  return ready ? "Ready to print." : "Loading every panel before printing."
}

type Props = {
  title: string
  context: string
  range: string
  variables: string | null
  imageName: string
  children: ReactNode
}

export function ExportSheet({
  title,
  context,
  range,
  variables,
  imageName,
  children,
}: Props) {
  const finish = useExportStore((state) => state.finish)
  const mode = useExportStore((state) => state.mode)
  const sheet = useRef<HTMLDivElement>(null)
  const [printed] = useState(() =>
    stampedAt(mode === "image" ? "Saved" : "Printed", new Date())
  )
  const [ready, setReady] = useState(false)

  useEffect(() => holdSheet(), [])

  useEffect(() => {
    const root = sheet.current
    if (!root || mode === null) return
    let active = true
    const printing = mode === "print"
    if (printing) window.addEventListener("afterprint", finish)
    void waitForPanels(root, { timeoutMs: settleTimeoutMs, settleMs }).then(
      async () => {
        if (!active) return
        setReady(true)
        if (printing) {
          window.print()
          return
        }
        try {
          await saveDashboardImage(root, imageName)
          toast.success(`Saved ${imageName}`)
        } catch {
          toast.error(
            "The dashboard could not be drawn as an image. Try Print or save as PDF."
          )
        }
        if (active) finish()
      }
    )
    return () => {
      active = false
      window.removeEventListener("afterprint", finish)
    }
  }, [finish, mode, imageName])

  return (
    <div className="export-sheet min-h-svh bg-chrome px-4 pb-8">
      <div className="export-bar sticky top-0 z-10 mx-auto flex max-w-[680px] flex-wrap items-center gap-2 bg-chrome py-3">
        <Button variant="outline" size="sm" onClick={finish}>
          <ArrowLeftIcon aria-hidden="true" />
          Back to dashboard
        </Button>
        {mode === "print" ? (
          <Button
            variant="outline"
            size="sm"
            disabled={!ready}
            onClick={() => window.print()}
          >
            <PrinterIcon aria-hidden="true" />
            Print or save as PDF
          </Button>
        ) : null}
        <p role="status" className="text-xs text-muted-foreground">
          {statusText(mode, ready)}
        </p>
      </div>
      <div className="export-scroll mx-auto max-w-[680px] overflow-x-auto">
        <div ref={sheet} style={{ width: sheetWidth }}>
          <header className="mb-3 flex flex-col gap-0.5 border-b pb-2">
            <h1 className="text-lg font-semibold">{title}</h1>
            <p className="text-xs text-muted-foreground">{context}</p>
            <p className="text-xs text-muted-foreground">{range}</p>
            {variables === null ? null : (
              <p className="text-xs text-muted-foreground">{variables}</p>
            )}
            <p className="text-xs text-muted-foreground">{printed}</p>
          </header>
          {children}
        </div>
      </div>
    </div>
  )
}
