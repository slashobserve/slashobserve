import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type FormEvent,
} from "react"
import { useNavigate } from "@tanstack/react-router"
import { CircleAlertIcon, FileJsonIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { handOffImport, type HandedImport } from "@/import/handoff"
import {
  MAX_FILE_BYTES,
  TOO_LARGE_MESSAGE,
  exceedsImportLimit,
} from "@/import/read-source"
import { cn } from "@/lib/utils"

import { IconTile } from "./home-section"

const textId = "home-import-json"

function hasFiles(event: DragEvent): boolean {
  return event.dataTransfer.types.includes("Files")
}

export function ImportStart({ autoFocus }: { autoFocus: boolean }) {
  const navigate = useNavigate()
  const jsonInput = useRef<HTMLInputElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const [text, setText] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    if (autoFocus) jsonInput.current?.focus()
  }, [autoFocus])

  function hand(next: HandedImport) {
    if (exceedsImportLimit(next.text)) {
      setProblem(TOO_LARGE_MESSAGE)
      return
    }
    handOffImport(next)
    void navigate({ to: "/import" })
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    const trimmed = text.trim()
    if (trimmed === "") return
    hand({ name: "Pasted JSON", text: trimmed, pasted: true })
  }

  async function takeFile(file: File | undefined) {
    if (file === undefined) return
    if (file.size > MAX_FILE_BYTES) {
      setProblem(TOO_LARGE_MESSAGE)
      return
    }
    hand({ name: file.name, text: await file.text(), pasted: false })
  }

  function onChosen(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ""
    void takeFile(file)
  }

  function onDragOver(event: DragEvent) {
    if (!hasFiles(event)) return
    event.preventDefault()
    setDragging(true)
  }

  function onDrop(event: DragEvent) {
    event.preventDefault()
    setDragging(false)
    void takeFile(event.dataTransfer.files[0])
  }

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={textId}>Grafana dashboard JSON</Label>
        <Input
          id={textId}
          ref={jsonInput}
          value={text}
          autoComplete="off"
          spellCheck={false}
          placeholder='{ "title": ... }'
          className="font-mono text-xs narrow:h-11"
          onChange={(event) => {
            setText(event.target.value)
            setProblem(null)
          }}
        />
      </div>
      <div
        onDragEnter={onDragOver}
        onDragOver={onDragOver}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          "flex flex-wrap items-center gap-3 rounded-card border bg-canvas p-4",
          dragging && "border-foreground"
        )}
      >
        <IconTile>
          <FileJsonIcon />
        </IconTile>
        <p className="min-w-0 flex-1 text-[13px]">
          Drop a Grafana dashboard JSON file here
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="narrow:h-11"
          onClick={() => fileInput.current?.click()}
        >
          Choose file
        </Button>
        <input
          ref={fileInput}
          type="file"
          accept=".json,application/json"
          aria-label="Grafana dashboard file"
          className="sr-only"
          tabIndex={-1}
          onChange={onChosen}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        Every panel is kept; what cannot render yet becomes a placeholder listed
        in the report.
      </p>
      {problem !== null ? (
        <p role="alert" className="flex items-start gap-2 text-[13px]">
          <CircleAlertIcon
            aria-hidden="true"
            className="mt-0.5 size-3.5 flex-none text-status-critical"
          />
          {problem}
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button
          type="submit"
          size="sm"
          className="narrow:h-11"
          disabled={text.trim() === ""}
        >
          Preview
        </Button>
      </div>
    </form>
  )
}
