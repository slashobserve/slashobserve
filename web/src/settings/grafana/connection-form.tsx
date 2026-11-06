import { useId, useRef, useState, type FormEvent } from "react"
import { LoaderCircleIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

import {
  connectionProblems,
  trimConnection,
  type ConnectionInput,
  type ConnectionProblems,
} from "./connection-problems"

type Props = {
  pending: boolean
  serverError: string | null
  onSubmit: (input: ConnectionInput) => Promise<boolean>
}

const empty: ConnectionInput = { name: "", url: "", token: "" }

const fields: {
  key: keyof ConnectionInput
  label: string
  placeholder: string
}[] = [
  { key: "name", label: "Name", placeholder: "Production" },
  {
    key: "url",
    label: "Grafana URL",
    placeholder: "https://grafana.example.com",
  },
  { key: "token", label: "Service account token", placeholder: "glsa_..." },
]

export function ConnectionForm({ pending, serverError, onSubmit }: Props) {
  const baseId = useId()
  const [values, setValues] = useState(empty)
  const [problems, setProblems] = useState<ConnectionProblems>({})
  const [saved, setSaved] = useState(false)
  const firstInput = useRef<HTMLInputElement>(null)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (pending) return
    const found = connectionProblems(values)
    setProblems(found)
    if (Object.keys(found).length > 0) return
    setSaved(false)
    if (await onSubmit(trimConnection(values))) {
      setValues(empty)
      setSaved(true)
      firstInput.current?.focus()
    }
  }

  return (
    <form
      noValidate
      onSubmit={(event) => void submit(event)}
      className="flex flex-col gap-3 px-4 py-3"
    >
      {fields.map((field) => {
        const inputId = `${baseId}-${field.key}`
        const errorId = `${inputId}-error`
        const problem = problems[field.key]
        return (
          <div key={field.key} className="flex flex-col gap-1">
            <Label htmlFor={inputId} className="text-[13px]">
              {field.label}
            </Label>
            <Input
              id={inputId}
              ref={field.key === "name" ? firstInput : undefined}
              type={field.key === "token" ? "password" : "text"}
              autoComplete="off"
              spellCheck={false}
              placeholder={field.placeholder}
              className={field.key === "name" ? undefined : "font-mono"}
              value={values[field.key]}
              aria-invalid={problem !== undefined}
              aria-describedby={problem === undefined ? undefined : errorId}
              onChange={(event) => {
                const next = event.target.value
                setValues((current) => ({ ...current, [field.key]: next }))
                setSaved(false)
              }}
            />
            {problem === undefined ? null : (
              <p id={errorId} className="text-xs text-destructive">
                {problem}
              </p>
            )}
          </div>
        )
      })}
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={pending} aria-busy={pending}>
          {pending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          Add connection
        </Button>
        {saved ? (
          <p role="status" className="text-xs text-muted-foreground">
            The token is stored encrypted and is never shown again.
          </p>
        ) : null}
      </div>
      {serverError === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{serverError}</AlertDescription>
        </Alert>
      )}
    </form>
  )
}
