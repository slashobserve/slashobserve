import { useId, useState, type FormEvent, type RefObject } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

import {
  connectionProblems,
  trimConnection,
  type ConnectionInput,
  type ConnectionProblems,
} from "./connection-problems"

type Props = {
  formId: string
  serverError: string | null
  onSubmit: (input: ConnectionInput) => void
  firstInputRef: RefObject<HTMLInputElement | null>
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

export function ConnectionForm({
  formId,
  serverError,
  onSubmit,
  firstInputRef,
}: Props) {
  const baseId = useId()
  const [values, setValues] = useState(empty)
  const [problems, setProblems] = useState<ConnectionProblems>({})

  function submit(event: FormEvent) {
    event.preventDefault()
    const found = connectionProblems(values)
    setProblems(found)
    if (Object.keys(found).length > 0) return
    onSubmit(trimConnection(values))
  }

  return (
    <form
      id={formId}
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-4"
    >
      {fields.map((field) => {
        const inputId = `${baseId}-${field.key}`
        const errorId = `${inputId}-error`
        const problem = problems[field.key]
        return (
          <div key={field.key} className="flex flex-col gap-1.5">
            <Label htmlFor={inputId} className="text-[13px] font-medium">
              {field.label}
            </Label>
            <Input
              id={inputId}
              ref={field.key === "name" ? firstInputRef : undefined}
              type={field.key === "token" ? "password" : "text"}
              autoComplete="off"
              spellCheck={false}
              placeholder={field.placeholder}
              className={cn("narrow:h-11", field.key !== "name" && "font-mono")}
              value={values[field.key]}
              aria-invalid={problem !== undefined}
              aria-describedby={problem === undefined ? undefined : errorId}
              onChange={(event) => {
                const next = event.target.value
                setValues((current) => ({ ...current, [field.key]: next }))
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
      {serverError === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{serverError}</AlertDescription>
        </Alert>
      )}
    </form>
  )
}
