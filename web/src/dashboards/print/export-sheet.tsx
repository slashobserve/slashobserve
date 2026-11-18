import { useEffect, useRef, useState, type ReactNode } from "react"
import { ArrowLeftIcon, PrinterIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

import { holdSheet, useExportStore } from "./export-store"
import { printedAt } from "./header-text"
import { sheetWidth } from "./print-grid"
import { waitForPanels } from "./settle"
import "./print.css"

const settleTimeoutMs = 30_000
const settleMs = 400

type Props = {
  title: string
  context: string
  range: string
  variables: string | null
  children: ReactNode
}

export function ExportSheet({
  title,
  context,
  range,
  variables,
  children,
}: Props) {
  const finish = useExportStore((state) => state.finish)
  const sheet = useRef<HTMLDivElement>(null)
  const [printed] = useState(() => printedAt(new Date()))
  const [ready, setReady] = useState(false)

  useEffect(() => holdSheet(), [])

  useEffect(() => {
    const root = sheet.current
    if (!root) return
    let active = true
    window.addEventListener("afterprint", finish)
    void waitForPanels(root, { timeoutMs: settleTimeoutMs, settleMs }).then(
      () => {
        if (!active) return
        setReady(true)
        window.print()
      }
    )
    return () => {
      active = false
      window.removeEventListener("afterprint", finish)
    }
  }, [finish])

  return (
    <div className="export-sheet min-h-svh bg-chrome px-4 pb-8">
      <div className="export-bar sticky top-0 z-10 mx-auto flex max-w-[680px] flex-wrap items-center gap-2 bg-chrome py-3">
        <Button variant="outline" size="sm" onClick={finish}>
          <ArrowLeftIcon aria-hidden="true" />
          Back to dashboard
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!ready}
          onClick={() => window.print()}
        >
          <PrinterIcon aria-hidden="true" />
          Print or save as PDF
        </Button>
        <p role="status" className="text-xs text-muted-foreground">
          {ready ? "Ready to print." : "Loading every panel before printing."}
        </p>
      </div>
      <div
        ref={sheet}
        style={{ width: sheetWidth }}
        className="mx-auto max-w-full"
      >
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
  )
}
