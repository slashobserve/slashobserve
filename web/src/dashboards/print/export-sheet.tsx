import { useEffect, useRef, useState, type ReactNode } from "react"
import { ArrowLeftIcon, ImageIcon, PrinterIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { holdLightTheme } from "@/lib/theme"

import { holdSheet, useExportStore } from "./export-store"
import { stampedAt } from "./header-text"
import { saveDashboardImage } from "./image"
import { sheetWidth } from "./print-grid"
import { slowPanelNames, waitForPanels } from "./settle"
import "./print.css"

const settleTimeoutMs = 30_000
const settleMs = 400

type Phase = "waiting" | "ready" | "slow"

function statusText(mode: string | null, phase: Phase): string {
  if (phase === "slow") return "Some panels are still loading."
  if (mode === "image") {
    return phase === "ready"
      ? "Drawing the image."
      : "Loading every panel before drawing."
  }
  return phase === "ready"
    ? "Ready to print."
    : "Loading every panel before printing."
}

async function saveImageAndLeave(
  root: HTMLElement,
  imageName: string,
  finish: () => void
) {
  try {
    await saveDashboardImage(root, imageName)
    toast.success(`Saved ${imageName}`)
  } catch {
    toast.error(
      "The dashboard could not be drawn as an image. Try Print or save as PDF."
    )
  }
  finish()
}

type Props = {
  title: string
  context: string
  range: string
  variables: string | null
  imageName: string
  timeoutMs?: number | undefined
  children: ReactNode
}

export function ExportSheet({
  title,
  context,
  range,
  variables,
  imageName,
  timeoutMs = settleTimeoutMs,
  children,
}: Props) {
  const finish = useExportStore((state) => state.finish)
  const mode = useExportStore((state) => state.mode)
  const sheet = useRef<HTMLDivElement>(null)
  const [printed] = useState(() =>
    stampedAt(mode === "image" ? "Saved" : "Printed", new Date())
  )
  const [phase, setPhase] = useState<Phase>("waiting")

  useEffect(() => holdSheet(), [])

  useEffect(() => {
    document.documentElement.dataset.export = ""
    return () => {
      delete document.documentElement.dataset.export
    }
  }, [])

  useEffect(() => (mode === "print" ? holdLightTheme() : undefined), [mode])

  useEffect(() => {
    const root = sheet.current
    if (!root || mode === null) return
    let active = true
    const printing = mode === "print"
    if (printing) window.addEventListener("afterprint", finish)
    void waitForPanels(root, { timeoutMs, settleMs }).then((settled) => {
      if (!active) return
      if (!settled) {
        setPhase("slow")
        toast.warning(
          `Still loading: ${slowPanelNames(root).join(", ")}. Use the button at the top to continue anyway.`
        )
        return
      }
      setPhase("ready")
      if (printing) window.print()
      else void saveImageAndLeave(root, imageName, finish)
    })
    return () => {
      active = false
      window.removeEventListener("afterprint", finish)
    }
  }, [finish, mode, imageName, timeoutMs])

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
            disabled={phase === "waiting"}
            onClick={() => window.print()}
          >
            <PrinterIcon aria-hidden="true" />
            Print or save as PDF
          </Button>
        ) : null}
        {mode === "image" && phase === "slow" ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              if (sheet.current) {
                void saveImageAndLeave(sheet.current, imageName, finish)
              }
            }}
          >
            <ImageIcon aria-hidden="true" />
            Save the image anyway
          </Button>
        ) : null}
        <p role="status" className="text-xs text-muted-foreground">
          {statusText(mode, phase)}
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
